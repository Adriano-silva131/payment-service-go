package usecase_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/adriano-linux/payment-service-go/internal/domain"
	"github.com/adriano-linux/payment-service-go/internal/usecase"
)

func TestReconcile_ResolvesApprovedAndPublishes(t *testing.T) {
	repo := newFakePaymentRepo()
	orderID := uuid.New()
	checkedOutPayment(repo, orderID, domain.PaymentMethodStripe, "cs_test_123")
	repo.byOrderID[orderID].CustomerID = "customer-1"

	stripe := &fakeGateway{
		method:          domain.PaymentMethodStripe,
		getStatusResult: &usecase.WebhookNotification{GatewayTransactionID: "cs_test_123", Status: domain.PaymentStatusApproved},
	}
	resolver := &fakeGatewayResolver{gateways: map[domain.PaymentMethod]usecase.PaymentGateway{domain.PaymentMethodStripe: stripe}}
	publisher := &fakePublisher{}

	uc := usecase.NewReconcile(repo, resolver, publisher)
	out, err := uc.Handle(context.Background(), usecase.ReconcileInput{OrderID: orderID, CustomerID: "customer-1"})

	require.NoError(t, err)
	assert.Equal(t, domain.PaymentStatusApproved, out.Status)
	require.Len(t, publisher.published, 1)
	assert.Equal(t, "payment.processed.v1", publisher.published[0].eventType)
}

func TestReconcile_AlreadyResolvedIsNoOpAndReportsStatus(t *testing.T) {
	repo := newFakePaymentRepo()
	orderID := uuid.New()
	checkedOutPayment(repo, orderID, domain.PaymentMethodStripe, "cs_test_123")
	repo.byOrderID[orderID].CustomerID = "customer-1"
	repo.byOrderID[orderID].Status = domain.PaymentStatusApproved

	resolver := &fakeGatewayResolver{gateways: map[domain.PaymentMethod]usecase.PaymentGateway{}}
	publisher := &fakePublisher{}

	uc := usecase.NewReconcile(repo, resolver, publisher)
	out, err := uc.Handle(context.Background(), usecase.ReconcileInput{OrderID: orderID, CustomerID: "customer-1"})

	require.NoError(t, err)
	assert.Equal(t, domain.PaymentStatusApproved, out.Status)
	assert.Empty(t, publisher.published, "a payment already resolved before reconcile was called must not republish")
}

func TestReconcile_StillPendingOnGatewayReturnsDistinctError(t *testing.T) {
	repo := newFakePaymentRepo()
	orderID := uuid.New()
	checkedOutPayment(repo, orderID, domain.PaymentMethodStripe, "cs_test_123")
	repo.byOrderID[orderID].CustomerID = "customer-1"

	stripe := &fakeGateway{method: domain.PaymentMethodStripe, getStatusResult: nil}
	resolver := &fakeGatewayResolver{gateways: map[domain.PaymentMethod]usecase.PaymentGateway{domain.PaymentMethodStripe: stripe}}
	publisher := &fakePublisher{}

	uc := usecase.NewReconcile(repo, resolver, publisher)
	_, err := uc.Handle(context.Background(), usecase.ReconcileInput{OrderID: orderID, CustomerID: "customer-1"})

	assert.ErrorIs(t, err, domain.ErrPaymentStillPending)
	assert.Empty(t, publisher.published)
}

func TestReconcile_CheckoutNeverStartedReturnsDistinctError(t *testing.T) {
	repo := newFakePaymentRepo()
	orderID := uuid.New()
	stagedPayment(repo, orderID, "customer-1") // still PENDING, no gateway/checkout session

	resolver := &fakeGatewayResolver{gateways: map[domain.PaymentMethod]usecase.PaymentGateway{}}
	uc := usecase.NewReconcile(repo, resolver, &fakePublisher{})

	_, err := uc.Handle(context.Background(), usecase.ReconcileInput{OrderID: orderID, CustomerID: "customer-1"})

	assert.ErrorIs(t, err, domain.ErrCheckoutNotStarted)
}

func TestReconcile_RejectsWhenCallerIsNotTheOrderOwner(t *testing.T) {
	repo := newFakePaymentRepo()
	orderID := uuid.New()
	checkedOutPayment(repo, orderID, domain.PaymentMethodStripe, "cs_test_123")
	repo.byOrderID[orderID].CustomerID = "customer-1"

	uc := usecase.NewReconcile(repo, &fakeGatewayResolver{gateways: map[domain.PaymentMethod]usecase.PaymentGateway{}}, &fakePublisher{})

	_, err := uc.Handle(context.Background(), usecase.ReconcileInput{OrderID: orderID, CustomerID: "someone-else"})

	assert.ErrorIs(t, err, domain.ErrForbidden)
}

func TestReconcile_ConcurrentCallsPublishOnlyOnce(t *testing.T) {
	repo := newFakePaymentRepo()
	orderID := uuid.New()
	checkedOutPayment(repo, orderID, domain.PaymentMethodStripe, "cs_test_123")
	repo.byOrderID[orderID].CustomerID = "customer-1"

	stripe := &fakeGateway{
		method:          domain.PaymentMethodStripe,
		getStatusResult: &usecase.WebhookNotification{GatewayTransactionID: "cs_test_123", Status: domain.PaymentStatusApproved},
	}
	resolver := &fakeGatewayResolver{gateways: map[domain.PaymentMethod]usecase.PaymentGateway{domain.PaymentMethodStripe: stripe}}
	publisher := &fakePublisher{}
	uc := usecase.NewReconcile(repo, resolver, publisher)

	for range 5 {
		_, err := uc.Handle(context.Background(), usecase.ReconcileInput{OrderID: orderID, CustomerID: "customer-1"})
		require.NoError(t, err)
	}

	assert.Len(t, publisher.published, 1, "repeated reconcile calls for an already-resolved payment must publish at most once")
}
