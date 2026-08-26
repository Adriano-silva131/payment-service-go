package handler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/adriano-linux/payment-service-go/internal/adapter/kafka/event"
	"github.com/adriano-linux/payment-service-go/internal/usecase"
)

// OrderCreatedHandler mirrors payment-service's OrderCreatedHandler.java: registers for
// order.created.v1 and delegates to the StagePayment use case.
type OrderCreatedHandler struct {
	stagePayment *usecase.StagePayment
}

func NewOrderCreatedHandler(stagePayment *usecase.StagePayment) *OrderCreatedHandler {
	return &OrderCreatedHandler{stagePayment: stagePayment}
}

func (h *OrderCreatedHandler) EventType() string {
	return event.OrderCreatedV1
}

func (h *OrderCreatedHandler) Handle(ctx context.Context, payload []byte) error {
	var evt event.OrderCreatedEvent
	if err := json.Unmarshal(payload, &evt); err != nil {
		return fmt.Errorf("unmarshalling order.created.v1: %w", err)
	}

	return h.stagePayment.Handle(ctx, usecase.StagePaymentInput{
		OrderID:       evt.OrderID,
		CustomerID:    evt.CustomerID,
		CustomerEmail: evt.CustomerEmail,
		TotalAmount:   evt.TotalAmount,
	})
}
