package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/adriano-linux/payment-service-go/internal/domain"
	"github.com/adriano-linux/payment-service-go/internal/usecase"
)

func stuckCheckoutPayment(repo *fakePaymentRepo, orderID uuid.UUID, gatewayTxID string, updatedAt time.Time) {
	method := domain.PaymentMethodStripe
	repo.byOrderID[orderID] = &domain.Payment{
		ID:                   uuid.New(),
		OrderID:              orderID,
		CustomerID:           "customer-1",
		CustomerEmail:        "customer@example.com",
		Amount:               decimal.NewFromInt(100),
		Status:               domain.PaymentStatusCheckoutStarted,
		Gateway:              &method,
		GatewayTransactionID: &gatewayTxID,
		CreatedAt:            updatedAt,
		UpdatedAt:            updatedAt,
	}
}

func TestReconcileSweep_ResolvesStaleCheckoutViaGatewayPoll(t *testing.T) {
	repo := newFakePaymentRepo()
	orderID := uuid.New()
	stuckCheckoutPayment(repo, orderID, "cs_test_123", time.Now().Add(-1*time.Hour))

	stripe := &fakeGateway{
		method:          domain.PaymentMethodStripe,
		getStatusResult: &usecase.WebhookNotification{GatewayTransactionID: "cs_test_123", Status: domain.PaymentStatusApproved},
	}
	resolver := &fakeGatewayResolver{gateways: map[domain.PaymentMethod]usecase.PaymentGateway{domain.PaymentMethodStripe: stripe}}

	sweep := usecase.NewReconcileSweep(repo, resolver, &fakePublisher{}, 2*time.Minute)
	sweep.Run(sweepOnceCtx(t), time.Millisecond)

	payment, err := repo.FindByOrderID(context.Background(), orderID)
	require.NoError(t, err)
	assert.Equal(t, domain.PaymentStatusApproved, payment.Status,
		"a payment stuck in CHECKOUT_STARTED past the stale threshold must be resolved by polling the gateway directly")
}

func TestReconcileSweep_LeavesFreshCheckoutsAlone(t *testing.T) {
	repo := newFakePaymentRepo()
	orderID := uuid.New()
	stuckCheckoutPayment(repo, orderID, "cs_test_123", time.Now())

	stripe := &fakeGateway{
		method:          domain.PaymentMethodStripe,
		getStatusResult: &usecase.WebhookNotification{GatewayTransactionID: "cs_test_123", Status: domain.PaymentStatusApproved},
	}
	resolver := &fakeGatewayResolver{gateways: map[domain.PaymentMethod]usecase.PaymentGateway{domain.PaymentMethodStripe: stripe}}

	sweep := usecase.NewReconcileSweep(repo, resolver, &fakePublisher{}, 2*time.Minute)
	sweep.Run(sweepOnceCtx(t), time.Millisecond)

	payment, err := repo.FindByOrderID(context.Background(), orderID)
	require.NoError(t, err)
	assert.Equal(t, domain.PaymentStatusCheckoutStarted, payment.Status,
		"a checkout still within the stale window must not be touched — the webhook may simply not have arrived yet")
}

func sweepOnceCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	t.Cleanup(cancel)
	return ctx
}
