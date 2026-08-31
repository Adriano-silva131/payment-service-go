package usecase

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/adriano-linux/payment-service-go/internal/domain"
)

const (
	PaymentEventsTopic     = "payment-events"
	PaymentProcessedV1Type = "payment.processed.v1"
)

type PaymentProcessedEvent struct {
	OrderID              string `json:"orderId"`
	Status               string `json:"status"`
	CustomerEmail        string `json:"customerEmail"`
	Gateway              string `json:"gateway,omitempty"`
	GatewayTransactionID string `json:"gatewayTransactionId,omitempty"`
}

type HandleWebhook struct {
	repo      PaymentRepository
	gateways  GatewayResolver
	publisher EventPublisher
}

func NewHandleWebhook(repo PaymentRepository, gateways GatewayResolver, publisher EventPublisher) *HandleWebhook {
	return &HandleWebhook{repo: repo, gateways: gateways, publisher: publisher}
}

func (uc *HandleWebhook) Handle(ctx context.Context, method domain.PaymentMethod, r *http.Request) error {
	gw, err := uc.gateways.Resolve(method)
	if err != nil {
		return err
	}

	notification, err := gw.ParseWebhook(ctx, r)
	if err != nil {
		return fmt.Errorf("parsing %s webhook: %w", method, err)
	}
	if notification == nil {
		// Valid webhook, but an event type this gateway adapter doesn't act on.
		return nil
	}

	var payment *domain.Payment
	if notification.OrderID != nil {
		payment, err = uc.repo.FindByOrderID(ctx, *notification.OrderID)
	} else {
		payment, err = uc.repo.FindByGatewayTransactionID(ctx, method, notification.GatewayTransactionID)
	}
	if err != nil {
		return fmt.Errorf("loading payment for %s notification: %w", method, err)
	}

	if payment.Status == domain.PaymentStatusApproved || payment.Status == domain.PaymentStatusRejected {
		// Already resolved (duplicate webhook delivery is expected/normal for both
		// Stripe and Mercado Pago) — treat as a no-op success, don't republish.
		return nil
	}

	payment.Status = notification.Status
	payment.UpdatedAt = time.Now().UTC()
	if notification.GatewayTransactionID != "" {
		txID := notification.GatewayTransactionID
		payment.GatewayTransactionID = &txID
	}

	if err := uc.repo.Update(ctx, payment); err != nil {
		return fmt.Errorf("updating payment status for order %s: %w", payment.OrderID, err)
	}

	event := PaymentProcessedEvent{
		OrderID:              payment.OrderID.String(),
		Status:               string(payment.Status),
		CustomerEmail:        payment.CustomerEmail,
		Gateway:              string(method),
		GatewayTransactionID: notification.GatewayTransactionID,
	}

	if err := uc.publisher.Publish(ctx, PaymentEventsTopic, payment.OrderID.String(), PaymentProcessedV1Type, event); err != nil {
		return fmt.Errorf("publishing payment.processed.v1 for order %s: %w", payment.OrderID, err)
	}

	return nil
}
