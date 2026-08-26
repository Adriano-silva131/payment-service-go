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
