package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
)

// Producer publishes JSON events with the same headers the Java services use
// (event-type, event-version, occurred-at), so existing Java consumers work unchanged.
type Producer struct {
	client *kgo.Client
}

func NewProducer(client *kgo.Client) *Producer {
	return &Producer{client: client}
}

func (p *Producer) Publish(ctx context.Context, topic, key, eventType string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling event payload for %s: %w", eventType, err)
	}

	headers := []kgo.RecordHeader{
		{Key: "event-type", Value: []byte(eventType)},
		{Key: "event-version", Value: []byte("1")},
		{Key: "occurred-at", Value: []byte(time.Now().UTC().Format(time.RFC3339Nano))},
	}
	// Injeta o traceparent do span ativo (HTTP handler ou consumer que chamou
	// Publish), pra quem consumir esse tópico poder continuar o mesmo trace.
	otel.GetTextMapPropagator().Inject(ctx, newHeaderCarrier(&headers))

	record := &kgo.Record{
		Topic:   topic,
		Key:     []byte(key),
		Value:   body,
		Headers: headers,
	}

	result := p.client.ProduceSync(ctx, record)
	if err := result.FirstErr(); err != nil {
		return fmt.Errorf("publishing to topic %s: %w", topic, err)
	}
	return nil
}

// PublishWithHeaders republishes a payload with a plain string header map — used by
// adapter/dlt.Reprocessor, which only knows built-in types (see that package's
// republisher interface), not this package's franz-go-specific RecordHeader type.
func (p *Producer) PublishWithHeaders(ctx context.Context, topic, key string, headers map[string]string, payload []byte) error {
	kgoHeaders := make([]kgo.RecordHeader, 0, len(headers))
	for k, v := range headers {
		kgoHeaders = append(kgoHeaders, kgo.RecordHeader{Key: k, Value: []byte(v)})
	}
	return p.PublishRaw(ctx, topic, key, kgoHeaders, payload)
}

// PublishRaw republishes an already-serialized payload (used by the retry/DLT flow, which
// only carries the original bytes forward rather than re-marshalling a typed struct).
func (p *Producer) PublishRaw(ctx context.Context, topic, key string, headers []kgo.RecordHeader, payload []byte) error {
	record := &kgo.Record{
		Topic:   topic,
		Key:     []byte(key),
		Value:   payload,
		Headers: headers,
	}
	result := p.client.ProduceSync(ctx, record)
	if err := result.FirstErr(); err != nil {
		return fmt.Errorf("republishing to topic %s: %w", topic, err)
	}
	return nil
}
