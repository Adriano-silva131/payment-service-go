package usecase_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/adriano-linux/payment-service-go/internal/domain"
	"github.com/adriano-linux/payment-service-go/internal/usecase"
)

type publishedEvent struct {
	topic, key, eventType string
	payload               any
}

type fakePublisher struct {
	published []publishedEvent
	err       error
}

func (f *fakePublisher) Publish(ctx context.Context, topic, key, eventType string, payload any) error {
	if f.err != nil {
		return f.err
	}
	f.published = append(f.published, publishedEvent{topic, key, eventType, payload})
	return nil
}

func checkedOutPayment(repo *fakePaymentRepo, orderID uuid.UUID, method domain.PaymentMethod, txID string) {
	m := method
	tx := txID
	repo.byOrderID[orderID] = &domain.Payment{
		ID:                   uuid.New(),
		OrderID:              orderID,
		CustomerEmail:        "customer@example.com",
		Amount:               decimal.NewFromInt(100),
		Status:               domain.PaymentStatusCheckoutStarted,
		Gateway:              &m,
		GatewayTransactionID: &tx,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}
}

func TestHandleWebhook_ApprovesAndPublishes(t *testing.T) {
	repo := newFakePaymentRepo()
	orderID := uuid.New()
	checkedOutPayment(repo, orderID, domain.PaymentMethodStripe, "cs_test_123")

	stripe := &fakeGateway{
		method: domain.PaymentMethodStripe,
		webhookResult: &usecase.WebhookNotification{
			GatewayTransactionID: "cs_test_123",
			Status:               domain.PaymentStatusApproved,
		},
	}
	resolver := &fakeGatewayResolver{gateways: map[domain.PaymentMethod]usecase.PaymentGateway{domain.PaymentMethodStripe: stripe}}
	publisher := &fakePublisher{}

	uc := usecase.NewHandleWebhook(repo, resolver, publisher)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", nil)

	err := uc.Handle(context.Background(), domain.PaymentMethodStripe, req)
	require.NoError(t, err)

	payment, err := repo.FindByOrderID(context.Background(), orderID)
	require.NoError(t, err)
	assert.Equal(t, domain.PaymentStatusApproved, payment.Status)

	require.Len(t, publisher.published, 1)
	assert.Equal(t, "payment-events", publisher.published[0].topic)
	assert.Equal(t, "payment.processed.v1", publisher.published[0].eventType)
	event := publisher.published[0].payload.(usecase.PaymentProcessedEvent)
	assert.Equal(t, "APPROVED", event.Status)
	assert.Equal(t, "customer@example.com", event.CustomerEmail)
}

func TestHandleWebhook_DuplicateDeliveryIsNoOp(t *testing.T) {
	repo := newFakePaymentRepo()
	orderID := uuid.New()
	checkedOutPayment(repo, orderID, domain.PaymentMethodStripe, "cs_test_123")
	repo.byOrderID[orderID].Status = domain.PaymentStatusApproved // already resolved

	stripe := &fakeGateway{
		method: domain.PaymentMethodStripe,
		webhookResult: &usecase.WebhookNotification{
			GatewayTransactionID: "cs_test_123",
			Status:               domain.PaymentStatusApproved,
		},
	}
	resolver := &fakeGatewayResolver{gateways: map[domain.PaymentMethod]usecase.PaymentGateway{domain.PaymentMethodStripe: stripe}}
	publisher := &fakePublisher{}

	uc := usecase.NewHandleWebhook(repo, resolver, publisher)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", nil)

	err := uc.Handle(context.Background(), domain.PaymentMethodStripe, req)
	require.NoError(t, err)
	assert.Empty(t, publisher.published, "duplicate webhook delivery must not republish")
}

func TestHandleWebhook_IgnoredEventTypeIsNoOp(t *testing.T) {
	repo := newFakePaymentRepo()
	stripe := &fakeGateway{method: domain.PaymentMethodStripe, webhookResult: nil, webhookErr: nil}
	resolver := &fakeGatewayResolver{gateways: map[domain.PaymentMethod]usecase.PaymentGateway{domain.PaymentMethodStripe: stripe}}
	publisher := &fakePublisher{}

	uc := usecase.NewHandleWebhook(repo, resolver, publisher)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", nil)

	err := uc.Handle(context.Background(), domain.PaymentMethodStripe, req)
	require.NoError(t, err)
	assert.Empty(t, publisher.published)
}

func TestHandleWebhook_ResolvesByOrderIDForMercadoPago(t *testing.T) {
	repo := newFakePaymentRepo()
	orderID := uuid.New()
	checkedOutPayment(repo, orderID, domain.PaymentMethodMercadoPago, "12345")

	mp := &fakeGateway{
		method: domain.PaymentMethodMercadoPago,
		webhookResult: &usecase.WebhookNotification{
			OrderID:              &orderID,
			GatewayTransactionID: "12345",
			Status:               domain.PaymentStatusRejected,
		},
	}
	resolver := &fakeGatewayResolver{gateways: map[domain.PaymentMethod]usecase.PaymentGateway{domain.PaymentMethodMercadoPago: mp}}
	publisher := &fakePublisher{}

	uc := usecase.NewHandleWebhook(repo, resolver, publisher)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/mercadopago", nil)

	err := uc.Handle(context.Background(), domain.PaymentMethodMercadoPago, req)
	require.NoError(t, err)

	payment, err := repo.FindByOrderID(context.Background(), orderID)
	require.NoError(t, err)
	assert.Equal(t, domain.PaymentStatusRejected, payment.Status)
}
