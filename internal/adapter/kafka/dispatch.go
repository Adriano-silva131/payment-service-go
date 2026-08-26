package kafka

import (
	"context"
	"fmt"
)

// EventHandler is dispatched by the event-type header, mirroring the Java
// GenericEventConsumer's EventHandler<T> interface + event-type-keyed bean registry.
type EventHandler interface {
	EventType() string
	Handle(ctx context.Context, payload []byte) error
}

// HandlerRegistry resolves an EventHandler by event-type, built once at startup from all
// registered handlers — the same "collect N implementations of one interface into a map"
// idiom used both here and by adapter/gateway.Registry.
type HandlerRegistry struct {
	handlers map[string]EventHandler
}

func NewHandlerRegistry(handlers ...EventHandler) *HandlerRegistry {
	m := make(map[string]EventHandler, len(handlers))
	for _, h := range handlers {
		m[h.EventType()] = h
	}
	return &HandlerRegistry{handlers: m}
}

var ErrNoHandlerRegistered = fmt.Errorf("no handler registered for event type")

func (r *HandlerRegistry) Dispatch(ctx context.Context, eventType string, payload []byte) error {
	h, ok := r.handlers[eventType]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNoHandlerRegistered, eventType)
	}
	return h.Handle(ctx, payload)
}
