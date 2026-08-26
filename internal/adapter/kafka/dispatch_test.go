package kafka

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubHandler struct {
	eventType string
	called    bool
	err       error
}

func (h *stubHandler) EventType() string { return h.eventType }

func (h *stubHandler) Handle(ctx context.Context, payload []byte) error {
	h.called = true
	return h.err
}

func TestHandlerRegistry_DispatchesByEventType(t *testing.T) {
	orderCreated := &stubHandler{eventType: "order.created.v1"}
	registry := NewHandlerRegistry(orderCreated)

	err := registry.Dispatch(context.Background(), "order.created.v1", []byte(`{}`))

	require.NoError(t, err)
	assert.True(t, orderCreated.called)
}

func TestHandlerRegistry_UnknownEventTypeErrors(t *testing.T) {
	registry := NewHandlerRegistry(&stubHandler{eventType: "order.created.v1"})

	err := registry.Dispatch(context.Background(), "unknown.event.v1", []byte(`{}`))

	assert.ErrorIs(t, err, ErrNoHandlerRegistered)
}

func TestHandlerRegistry_PropagatesHandlerError(t *testing.T) {
	boom := errors.New("boom")
	registry := NewHandlerRegistry(&stubHandler{eventType: "order.created.v1", err: boom})

	err := registry.Dispatch(context.Background(), "order.created.v1", []byte(`{}`))

	assert.ErrorIs(t, err, boom)
}
