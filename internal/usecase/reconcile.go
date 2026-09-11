package usecase

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/adriano-linux/payment-service-go/internal/domain"
)

type ReconcileInput struct {
	OrderID    uuid.UUID
	CustomerID string
}

type ReconcileOutput struct {
	Status domain.PaymentStatus
}

type Reconcile struct {
	repo      PaymentRepository
	gateways  GatewayResolver
	publisher EventPublisher
}

func NewReconcile(repo PaymentRepository, gateways GatewayResolver, publisher EventPublisher) *Reconcile {
	return &Reconcile{repo: repo, gateways: gateways, publisher: publisher}
}

func (uc *Reconcile) Handle(ctx context.Context, in ReconcileInput) (*ReconcileOutput, error) {
	payment, err := uc.repo.FindByOrderID(ctx, in.OrderID)
	if err != nil {
		return nil, fmt.Errorf("loading payment for order %s: %w", in.OrderID, err)
	}

	if payment.CustomerID != in.CustomerID {
		return nil, domain.ErrForbidden
	}

	if payment.Status == domain.PaymentStatusApproved || payment.Status == domain.PaymentStatusRejected {
		return &ReconcileOutput{Status: payment.Status}, nil
	}
	if payment.Gateway == nil || payment.GatewayTransactionID == nil {
		return nil, domain.ErrCheckoutNotStarted
	}

	gw, err := uc.gateways.Resolve(*payment.Gateway)
	if err != nil {
		return nil, err
	}

	notification, err := gw.GetStatus(ctx, payment.OrderID, *payment.GatewayTransactionID)
	if err != nil {
		return nil, fmt.Errorf("checking %s status for order %s: %w", *payment.Gateway, in.OrderID, err)
	}
	if notification == nil {
		return nil, domain.ErrPaymentStillPending
	}

	if err := resolvePayment(ctx, uc.repo, uc.publisher, *payment.Gateway, notification); err != nil {
		return nil, err
	}

	return &ReconcileOutput{Status: notification.Status}, nil
}
