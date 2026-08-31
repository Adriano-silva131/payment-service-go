package usecase

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/adriano-linux/payment-service-go/internal/domain"
)

type PaymentRepository interface {
	ExistsByOrderID(ctx context.Context, orderID uuid.UUID) (bool, error)
	Insert(ctx context.Context, p *domain.Payment) error
	FindByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.Payment, error)
	FindByGatewayTransactionID(ctx context.Context, gateway domain.PaymentMethod, txID string) (*domain.Payment, error)
	Update(ctx context.Context, p *domain.Payment) error
	// TryClaimForCheckout atomically moves the payment from PENDING to CHECKOUT_STARTED,
	// so concurrent StartCheckout calls for the same order can't both reach the gateway.
	// Reports false (no error) if the payment wasn't PENDING — someone else already claimed it.
	TryClaimForCheckout(ctx context.Context, orderID uuid.UUID) (bool, error)
	// ReleaseCheckoutClaim reverts a CHECKOUT_STARTED payment back to PENDING, used when the
	// gateway call after a successful claim fails, so a later request can retry.
	ReleaseCheckoutClaim(ctx context.Context, orderID uuid.UUID) error
}

type DltRepository interface {
	ExistsPendingByTopicAndKey(ctx context.Context, topic, key string) (bool, error)
	Insert(ctx context.Context, msg *domain.DltMessage) error
	ProcessPendingLocked(ctx context.Context, limit int, fn func(ctx context.Context, msg *domain.DltMessage) error) error
}

type EventPublisher interface {
	Publish(ctx context.Context, topic, key, eventType string, payload any) error
}

type CheckoutRequest struct {
	OrderID       uuid.UUID
	Amount        decimal.Decimal
	CustomerEmail string
	SuccessURL    string
	CancelURL     string
	// IdempotencyKey is stable per order, so a retried CreateCheckout call (client retry,
	// crash-and-retry after the DB claim but before the gateway response was recorded)
	// resolves to the same gateway-side session instead of creating a duplicate one.
	IdempotencyKey string
}

type CheckoutResult struct {
	GatewayTransactionID string
	CheckoutURL          string
}

type WebhookNotification struct {
	OrderID              *uuid.UUID
	GatewayTransactionID string
	Status               domain.PaymentStatus
}

type PaymentGateway interface {
	Method() domain.PaymentMethod
	CreateCheckout(ctx context.Context, req CheckoutRequest) (*CheckoutResult, error)
	ParseWebhook(ctx context.Context, r *http.Request) (*WebhookNotification, error)
}
