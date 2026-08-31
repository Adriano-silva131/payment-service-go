package usecase_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/adriano-linux/payment-service-go/internal/domain"
	"github.com/adriano-linux/payment-service-go/internal/usecase"
)

type fakeGateway struct {
	method         domain.PaymentMethod
	checkoutResult *usecase.CheckoutResult
	checkoutErr    error
	webhookResult  *usecase.WebhookNotification
	webhookErr     error
	lastCheckout   usecase.CheckoutRequest
}

func (g *fakeGateway) Method() domain.PaymentMethod { return g.method }

func (g *fakeGateway) CreateCheckout(ctx context.Context, req usecase.CheckoutRequest) (*usecase.CheckoutResult, error) {
	g.lastCheckout = req
	return g.checkoutResult, g.checkoutErr
}

func (g *fakeGateway) ParseWebhook(ctx context.Context, r *http.Request) (*usecase.WebhookNotification, error) {
	return g.webhookResult, g.webhookErr
}

type fakeGatewayResolver struct {
	gateways map[domain.PaymentMethod]usecase.PaymentGateway
}

func (r *fakeGatewayResolver) Resolve(method domain.PaymentMethod) (usecase.PaymentGateway, error) {
	gw, ok := r.gateways[method]
	if !ok {
		return nil, domain.ErrUnsupportedGateway
	}
	return gw, nil
}

func stagedPayment(repo *fakePaymentRepo, orderID uuid.UUID, customerID string) {
	repo.byOrderID[orderID] = &domain.Payment{
		ID:            uuid.New(),
		OrderID:       orderID,
		CustomerID:    customerID,
		CustomerEmail: "customer@example.com",
		Amount:        decimal.NewFromInt(100),
		Status:        domain.PaymentStatusPending,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

func TestStartCheckout_CreatesSessionAndPersistsGateway(t *testing.T) {
	repo := newFakePaymentRepo()
	orderID := uuid.New()
	stagedPayment(repo, orderID, "customer-1")

	stripe := &fakeGateway{
		method:         domain.PaymentMethodStripe,
		checkoutResult: &usecase.CheckoutResult{GatewayTransactionID: "cs_test_123", CheckoutURL: "https://checkout.stripe.com/cs_test_123"},
	}
	resolver := &fakeGatewayResolver{gateways: map[domain.PaymentMethod]usecase.PaymentGateway{domain.PaymentMethodStripe: stripe}}

	uc := usecase.NewStartCheckout(repo, resolver, "https://success", "https://cancel")
	out, err := uc.Handle(context.Background(), usecase.StartCheckoutInput{OrderID: orderID, Method: domain.PaymentMethodStripe, CustomerID: "customer-1"})

	require.NoError(t, err)
	assert.Equal(t, "https://checkout.stripe.com/cs_test_123", out.CheckoutURL)

	payment, err := repo.FindByOrderID(context.Background(), orderID)
	require.NoError(t, err)
	require.NotNil(t, payment.Gateway)
	assert.Equal(t, domain.PaymentMethodStripe, *payment.Gateway)
	assert.Equal(t, "cs_test_123", *payment.GatewayTransactionID)
}

func TestStartCheckout_FailsWhenPaymentNotPending(t *testing.T) {
	repo := newFakePaymentRepo()
	orderID := uuid.New()
	stagedPayment(repo, orderID, "customer-1")
	repo.byOrderID[orderID].Status = domain.PaymentStatusApproved

	resolver := &fakeGatewayResolver{gateways: map[domain.PaymentMethod]usecase.PaymentGateway{}}
	uc := usecase.NewStartCheckout(repo, resolver, "https://success", "https://cancel")

	_, err := uc.Handle(context.Background(), usecase.StartCheckoutInput{OrderID: orderID, Method: domain.PaymentMethodStripe, CustomerID: "customer-1"})
	assert.Error(t, err)
}

func TestStartCheckout_UnknownOrderReturnsNotFound(t *testing.T) {
	repo := newFakePaymentRepo()
	resolver := &fakeGatewayResolver{gateways: map[domain.PaymentMethod]usecase.PaymentGateway{}}
	uc := usecase.NewStartCheckout(repo, resolver, "https://success", "https://cancel")

	_, err := uc.Handle(context.Background(), usecase.StartCheckoutInput{OrderID: uuid.New(), Method: domain.PaymentMethodStripe, CustomerID: "customer-1"})
	assert.ErrorIs(t, err, domain.ErrPaymentNotFound)
}

func TestStartCheckout_RejectsWhenCallerIsNotTheOrderOwner(t *testing.T) {
	repo := newFakePaymentRepo()
	orderID := uuid.New()
	stagedPayment(repo, orderID, "customer-1")

	resolver := &fakeGatewayResolver{gateways: map[domain.PaymentMethod]usecase.PaymentGateway{}}
	uc := usecase.NewStartCheckout(repo, resolver, "https://success", "https://cancel")

	_, err := uc.Handle(context.Background(), usecase.StartCheckoutInput{OrderID: orderID, Method: domain.PaymentMethodStripe, CustomerID: "someone-else"})
	assert.ErrorIs(t, err, domain.ErrForbidden)
}

func TestStartCheckout_SendsStableIdempotencyKeyToGateway(t *testing.T) {
	repo := newFakePaymentRepo()
	orderID := uuid.New()
	stagedPayment(repo, orderID, "customer-1")

	stripe := &fakeGateway{
		method:         domain.PaymentMethodStripe,
		checkoutResult: &usecase.CheckoutResult{GatewayTransactionID: "cs_test_123", CheckoutURL: "https://checkout.stripe.com/cs_test_123"},
	}
	resolver := &fakeGatewayResolver{gateways: map[domain.PaymentMethod]usecase.PaymentGateway{domain.PaymentMethodStripe: stripe}}

	uc := usecase.NewStartCheckout(repo, resolver, "https://success", "https://cancel")
	_, err := uc.Handle(context.Background(), usecase.StartCheckoutInput{OrderID: orderID, Method: domain.PaymentMethodStripe, CustomerID: "customer-1"})

	require.NoError(t, err)
	assert.Equal(t, orderID.String(), stripe.lastCheckout.IdempotencyKey, "idempotency key must be stable per order, so a retried call resolves to the same gateway session")
}

func TestStartCheckout_ConcurrentCallForSameOrderIsRejectedNotDoubleCharged(t *testing.T) {
	repo := newFakePaymentRepo()
	orderID := uuid.New()
	stagedPayment(repo, orderID, "customer-1")

	// Simulates a second request winning the race after the first already claimed the order.
	claimed, err := repo.TryClaimForCheckout(context.Background(), orderID)
	require.NoError(t, err)
	require.True(t, claimed)

	resolver := &fakeGatewayResolver{gateways: map[domain.PaymentMethod]usecase.PaymentGateway{}}
	uc := usecase.NewStartCheckout(repo, resolver, "https://success", "https://cancel")

	_, err = uc.Handle(context.Background(), usecase.StartCheckoutInput{OrderID: orderID, Method: domain.PaymentMethodStripe, CustomerID: "customer-1"})
	assert.ErrorIs(t, err, domain.ErrCheckoutInProgress)
}

func TestStartCheckout_ReleasesClaimWhenPersistingResultFails(t *testing.T) {
	repo := newFakePaymentRepo()
	orderID := uuid.New()
	stagedPayment(repo, orderID, "customer-1")
	repo.updateErr = assert.AnError

	stripe := &fakeGateway{
		method:         domain.PaymentMethodStripe,
		checkoutResult: &usecase.CheckoutResult{GatewayTransactionID: "cs_test_123", CheckoutURL: "https://checkout.stripe.com/cs_test_123"},
	}
	resolver := &fakeGatewayResolver{gateways: map[domain.PaymentMethod]usecase.PaymentGateway{domain.PaymentMethodStripe: stripe}}
	uc := usecase.NewStartCheckout(repo, resolver, "https://success", "https://cancel")

	_, err := uc.Handle(context.Background(), usecase.StartCheckoutInput{OrderID: orderID, Method: domain.PaymentMethodStripe, CustomerID: "customer-1"})
	require.Error(t, err)

	assert.Equal(t, domain.PaymentStatusPending, repo.byOrderID[orderID].Status,
		"a gateway session was created but never persisted — the claim must still be released so the order isn't stuck forever")
}

func TestStartCheckout_AlreadyResolvedPaymentReturnsDistinctError(t *testing.T) {
	repo := newFakePaymentRepo()
	orderID := uuid.New()
	stagedPayment(repo, orderID, "customer-1")
	repo.byOrderID[orderID].Status = domain.PaymentStatusApproved

	resolver := &fakeGatewayResolver{gateways: map[domain.PaymentMethod]usecase.PaymentGateway{}}
	uc := usecase.NewStartCheckout(repo, resolver, "https://success", "https://cancel")

	_, err := uc.Handle(context.Background(), usecase.StartCheckoutInput{OrderID: orderID, Method: domain.PaymentMethodStripe, CustomerID: "customer-1"})
	assert.ErrorIs(t, err, domain.ErrPaymentAlreadyResolved,
		"an already-approved/rejected order must not be reported as a transient checkout race")
}

func TestStartCheckout_ReleasesClaimWhenGatewayCallFails(t *testing.T) {
	repo := newFakePaymentRepo()
	orderID := uuid.New()
	stagedPayment(repo, orderID, "customer-1")

	stripe := &fakeGateway{method: domain.PaymentMethodStripe, checkoutErr: assert.AnError}
	resolver := &fakeGatewayResolver{gateways: map[domain.PaymentMethod]usecase.PaymentGateway{domain.PaymentMethodStripe: stripe}}
	uc := usecase.NewStartCheckout(repo, resolver, "https://success", "https://cancel")

	_, err := uc.Handle(context.Background(), usecase.StartCheckoutInput{OrderID: orderID, Method: domain.PaymentMethodStripe, CustomerID: "customer-1"})
	require.Error(t, err)

	payment, err := repo.FindByOrderID(context.Background(), orderID)
	require.NoError(t, err)
	assert.Equal(t, domain.PaymentStatusPending, payment.Status, "a failed gateway call must release the claim so the order can be retried")
}
