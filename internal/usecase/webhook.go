package usecase

import (
	"context"
	"fmt"
	"net/http"

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

	return resolvePayment(ctx, uc.repo, uc.publisher, method, notification)
}

func resolvePayment(ctx context.Context, repo PaymentRepository, publisher EventPublisher, method domain.PaymentMethod, notification *WebhookNotification) error {
	var payment *domain.Payment
	var err error
	if notification.OrderID != nil {
		payment, err = repo.FindByOrderID(ctx, *notification.OrderID)
	} else {
		payment, err = repo.FindByGatewayTransactionID(ctx, method, notification.GatewayTransactionID)
	}
	if err != nil {
		return fmt.Errorf("loading payment for %s notification: %w", method, err)
	}

	if payment.Status == domain.PaymentStatusApproved || payment.Status == domain.PaymentStatusRejected {
		return nil
	}

	resolved, err := repo.ResolveIfNotFinal(ctx, payment.OrderID, notification.Status, method, notification.GatewayTransactionID)
	if err != nil {
		return fmt.Errorf("resolving payment status for order %s: %w", payment.OrderID, err)
	}
	if !resolved {
		// Lost the race to a concurrent caller that resolved this payment first — it already
		// published the event, so publishing again here would be a duplicate.
		return nil
	}

	event := PaymentProcessedEvent{
		OrderID:              payment.OrderID.String(),
		Status:               string(notification.Status),
		CustomerEmail:        payment.CustomerEmail,
		Gateway:              string(method),
		GatewayTransactionID: notification.GatewayTransactionID,
	}

	if err := publisher.Publish(ctx, PaymentEventsTopic, payment.OrderID.String(), PaymentProcessedV1Type, event); err != nil {
		return fmt.Errorf("publishing payment.processed.v1 for order %s: %w", payment.OrderID, err)
	}

	return nil
}
