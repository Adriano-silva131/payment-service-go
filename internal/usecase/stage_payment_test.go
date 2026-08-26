package usecase_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/adriano-linux/payment-service-go/internal/domain"
	"github.com/adriano-linux/payment-service-go/internal/usecase"
)

type fakePaymentRepo struct {
	byOrderID map[uuid.UUID]*domain.Payment
	insertErr error
}

func newFakePaymentRepo() *fakePaymentRepo {
	return &fakePaymentRepo{byOrderID: make(map[uuid.UUID]*domain.Payment)}
}

func (f *fakePaymentRepo) ExistsByOrderID(ctx context.Context, orderID uuid.UUID) (bool, error) {
	_, ok := f.byOrderID[orderID]
	return ok, nil
}

func (f *fakePaymentRepo) Insert(ctx context.Context, p *domain.Payment) error {
	if f.insertErr != nil {
		return f.insertErr
	}
	f.byOrderID[p.OrderID] = p
	return nil
}

func (f *fakePaymentRepo) FindByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.Payment, error) {
	p, ok := f.byOrderID[orderID]
	if !ok {
		return nil, domain.ErrPaymentNotFound
	}
	return p, nil
}

func (f *fakePaymentRepo) FindByGatewayTransactionID(ctx context.Context, gateway domain.PaymentMethod, txID string) (*domain.Payment, error) {
	for _, p := range f.byOrderID {
		if p.Gateway != nil && *p.Gateway == gateway && p.GatewayTransactionID != nil && *p.GatewayTransactionID == txID {
			return p, nil
		}
	}
	return nil, domain.ErrPaymentNotFound
}

func (f *fakePaymentRepo) Update(ctx context.Context, p *domain.Payment) error {
	if _, ok := f.byOrderID[p.OrderID]; !ok {
		return domain.ErrPaymentNotFound
	}
	f.byOrderID[p.OrderID] = p
	return nil
}

func TestStagePayment_CreatesPendingPayment(t *testing.T) {
	repo := newFakePaymentRepo()
	uc := usecase.NewStagePayment(repo)
	orderID := uuid.New()

	err := uc.Handle(context.Background(), usecase.StagePaymentInput{
		OrderID:       orderID,
		CustomerID:    "customer-1",
		CustomerEmail: "customer@example.com",
		TotalAmount:   decimal.NewFromInt(150),
	})

	require.NoError(t, err)
	payment, err := repo.FindByOrderID(context.Background(), orderID)
	require.NoError(t, err)
	assert.Equal(t, domain.PaymentStatusPending, payment.Status)
	assert.Equal(t, "customer@example.com", payment.CustomerEmail)
	assert.True(t, payment.Amount.Equal(decimal.NewFromInt(150)))
}

func TestStagePayment_IsIdempotent(t *testing.T) {
	repo := newFakePaymentRepo()
	uc := usecase.NewStagePayment(repo)
	orderID := uuid.New()
	input := usecase.StagePaymentInput{OrderID: orderID, CustomerID: "c1", CustomerEmail: "a@b.com", TotalAmount: decimal.NewFromInt(10)}

	require.NoError(t, uc.Handle(context.Background(), input))
	require.NoError(t, uc.Handle(context.Background(), input))

	payment, err := repo.FindByOrderID(context.Background(), orderID)
	require.NoError(t, err)
	assert.Equal(t, orderID, payment.OrderID)
}
