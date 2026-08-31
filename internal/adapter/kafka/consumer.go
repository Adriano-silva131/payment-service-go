package kafka

import (
	"context"
	"log/slog"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Consumer polls a single topic in a consumer group and dispatches each record by its
// event-type header through a HandlerRegistry. On handler failure it republishes the
// record onto the retry topic (attempt 1) instead of retrying in place, so a single slow
// or broken message never blocks the partition — mirroring why @RetryableTopic exists.
type Consumer struct {
	client   *kgo.Client
	topic    string
	registry *HandlerRegistry
	producer *Producer
}

func NewConsumer(client *kgo.Client, topic string, registry *HandlerRegistry, producer *Producer) *Consumer {
	return &Consumer{client: client, topic: topic, registry: registry, producer: producer}
}

func (c *Consumer) Run(ctx context.Context) {
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

func (c *Consumer) handleRecord(ctx context.Context, r *kgo.Record) {
	// Extrai o traceparent do produtor (se houver) e abre um span filho, pra
	// esse consumo aparecer conectado ao trace de quem publicou a mensagem
	// em vez de começar um trace novo e desconectado.
	carrierHeaders := r.Headers
	ctx = otel.GetTextMapPropagator().Extract(ctx, newHeaderCarrier(&carrierHeaders))
	ctx, span := otel.Tracer("payment-service-kafka").Start(ctx, "kafka.consume "+r.Topic,
		trace.WithSpanKind(trace.SpanKindConsumer))
	defer span.End()

	eventType := EventTypeFromHeaders(r.Headers)

	err := c.registry.Dispatch(ctx, eventType, r.Value)
	if err == nil {
		return
	}

	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())

	slog.ErrorContext(ctx, "handler failed, sending to retry topic",
		"topic", r.Topic, "eventType", eventType, "error", err)

	retryTopic := RetryTopic(r.Topic)
	headers := BuildRetryHeaders(r.Topic, string(r.Key), eventType, 1)
	headers = append(headers, traceHeadersFrom(r.Headers)...)
	if pubErr := c.producer.PublishRaw(ctx, retryTopic, string(r.Key), headers, r.Value); pubErr != nil {
		slog.ErrorContext(ctx, "failed to publish to retry topic, message may be lost",
			"topic", retryTopic, "error", pubErr)
	}
}
