package kafka

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/adriano-linux/payment-service-go/internal/domain"
	"github.com/adriano-linux/payment-service-go/internal/usecase"
)

// RetryConsumer polls the retry topic, waits out each record's backoff window, and either
// re-dispatches successfully, republishes for the next attempt, or (after MaxAttempts)
// writes the message to dlt_messages — mirroring GenericEventConsumer's @RetryableTopic +
// @DltHandler pair, minus Spring Kafka's per-attempt topic suffixing.
type RetryConsumer struct {
	client   *kgo.Client
	topic    string
	registry *HandlerRegistry
	producer *Producer
	dltRepo  usecase.DltRepository
}

func NewRetryConsumer(client *kgo.Client, topic string, registry *HandlerRegistry, producer *Producer, dltRepo usecase.DltRepository) *RetryConsumer {
	return &RetryConsumer{client: client, topic: topic, registry: registry, producer: producer, dltRepo: dltRepo}
}

func (c *RetryConsumer) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		fetches := c.client.PollFetches(ctx)
		if fetches.IsClientClosed() {
			return
		}

		fetches.EachError(func(topic string, partition int32, err error) {
			slog.ErrorContext(ctx, "kafka fetch error", "topic", topic, "partition", partition, "error", err)
		})

		fetches.EachRecord(func(r *kgo.Record) {
			c.handleRecord(ctx, r)
		})

		if err := c.client.CommitUncommittedOffsets(ctx); err != nil {
			slog.ErrorContext(ctx, "committing offsets", "topic", c.topic, "error", err)
		}
	}
}

func (c *RetryConsumer) handleRecord(ctx context.Context, r *kgo.Record) {
	notBefore := NotBeforeFromHeaders(r.Headers)
	if wait := time.Until(notBefore); wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return
		}
	}

	eventType := EventTypeFromHeaders(r.Headers)
	originalTopic, _ := headerValue(r.Headers, HeaderOriginalTopic)
	originalKey, _ := headerValue(r.Headers, HeaderOriginalKey)
	attempt := AttemptFromHeaders(r.Headers)

	err := c.registry.Dispatch(ctx, eventType, r.Value)
	if err == nil {
		return
	}

	nextAttempt := attempt + 1
	if nextAttempt <= MaxAttempts {
		slog.WarnContext(ctx, "retry attempt failed, scheduling next attempt",
			"eventType", eventType, "attempt", nextAttempt, "error", err)
		headers := BuildRetryHeaders(originalTopic, originalKey, eventType, nextAttempt)
		if pubErr := c.producer.PublishRaw(ctx, c.topic, originalKey, headers, r.Value); pubErr != nil {
			slog.ErrorContext(ctx, "failed to republish retry, message may be lost", "error", pubErr)
		}
		return
	}

	c.sendToDlt(ctx, originalTopic, originalKey, eventType, r.Value, err)
}

func (c *RetryConsumer) sendToDlt(ctx context.Context, originalTopic, key, eventType string, payload []byte, cause error) {
	exists, err := c.dltRepo.ExistsPendingByTopicAndKey(ctx, originalTopic, key)
	if err != nil {
		slog.ErrorContext(ctx, "checking existing dlt entry", "error", err)
		return
	}
	if exists {
		slog.WarnContext(ctx, "dlt entry already pending for this topic+key, skipping duplicate",
			"topic", originalTopic, "key", key)
		return
	}

	msg := &domain.DltMessage{
		ID:            uuid.New(),
		OriginalTopic: originalTopic,
		MessageKey:    key,
		EventType:     eventType,
		Payload:       string(payload),
		ErrorMessage:  cause.Error(),
		Status:        domain.DltStatusPending,
		CreatedAt:     time.Now().UTC(),
	}

	if err := c.dltRepo.Insert(ctx, msg); err != nil {
		slog.ErrorContext(ctx, "failed to persist dlt message, message lost", "error", err)
		return
	}

	slog.ErrorContext(ctx, "message exhausted retries, sent to DLT",
		"topic", originalTopic, "eventType", eventType)
}
