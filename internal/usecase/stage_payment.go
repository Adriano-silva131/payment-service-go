package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/adriano-linux/payment-service-go/internal/domain"
)

type StagePaymentInput struct {
	OrderID       uuid.UUID
	CustomerID    string
	CustomerEmail string
	TotalAmount   decimal.Decimal
}

type StagePayment struct {
	repo PaymentRepository
}

func NewStagePayment(repo PaymentRepository) *StagePayment {
	return &StagePayment{repo: repo}
}

func (uc *StagePayment) Handle(ctx context.Context, in StagePaymentInput) error {
	exists, err := uc.repo.ExistsByOrderID(ctx, in.OrderID)
	if err != nil {
		return fmt.Errorf("checking existing payment for order %s: %w", in.OrderID, err)
	}
	if exists {
		slog.InfoContext(ctx, "payment already staged, skipping", "orderId", in.OrderID)
		return nil
	}

	now := time.Now().UTC()
	payment := &domain.Payment{
		ID:            uuid.New(),
		OrderID:       in.OrderID,
		CustomerID:    in.CustomerID,
		CustomerEmail: in.CustomerEmail,
		Amount:        in.TotalAmount,
		Status:        domain.PaymentStatusPending,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := uc.repo.Insert(ctx, payment); err != nil {
		return fmt.Errorf("inserting staged payment for order %s: %w", in.OrderID, err)
	}

	slog.InfoContext(ctx, "payment staged as PENDING", "orderId", in.OrderID)
	return nil
}
