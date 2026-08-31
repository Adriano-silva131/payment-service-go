package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/adriano-linux/payment-service-go/internal/domain"
)

type GatewayResolver interface {
	Resolve(method domain.PaymentMethod) (PaymentGateway, error)
}

type StartCheckoutInput struct {
	OrderID    uuid.UUID
	Method     domain.PaymentMethod
	CustomerID string
}

type StartCheckoutOutput struct {
	CheckoutURL string
}

type StartCheckout struct {
	repo       PaymentRepository
	gateways   GatewayResolver
	successURL string
	cancelURL  string
}

func NewStartCheckout(repo PaymentRepository, gateways GatewayResolver, successURL, cancelURL string) *StartCheckout {
	return &StartCheckout{repo: repo, gateways: gateways, successURL: successURL, cancelURL: cancelURL}
}

func (uc *StartCheckout) Handle(ctx context.Context, in StartCheckoutInput) (*StartCheckoutOutput, error) {
	payment, err := uc.repo.FindByOrderID(ctx, in.OrderID)
	if err != nil {
		if errors.Is(err, domain.ErrPaymentNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("loading payment for order %s: %w", in.OrderID, err)
	}

	if payment.CustomerID != in.CustomerID {
		return nil, domain.ErrForbidden
	}

	claimed, err := uc.repo.TryClaimForCheckout(ctx, in.OrderID)
	if err != nil {
		return nil, fmt.Errorf("claiming payment for checkout, order %s: %w", in.OrderID, err)
	}
	if !claimed {
		if payment.Status == domain.PaymentStatusApproved || payment.Status == domain.PaymentStatusRejected {
			return nil, domain.ErrPaymentAlreadyResolved
		}
		return nil, domain.ErrCheckoutInProgress
	}

	// From here on, any failure must release the claim back to PENDING so the order
	// isn't stuck in CHECKOUT_STARTED forever with no usable checkout URL.
	succeeded := false
	defer func() {
		if succeeded {
			return
		}
		if releaseErr := uc.repo.ReleaseCheckoutClaim(ctx, in.OrderID); releaseErr != nil {
			slog.ErrorContext(ctx, "failed to release checkout claim after error", "orderId", in.OrderID, "error", releaseErr)
		}
	}()

	gw, err := uc.gateways.Resolve(in.Method)
	if err != nil {
		return nil, err
	}

	result, err := gw.CreateCheckout(ctx, CheckoutRequest{
		OrderID:        payment.OrderID,
		Amount:         payment.Amount,
		CustomerEmail:  payment.CustomerEmail,
		SuccessURL:     uc.successURL,
		CancelURL:      uc.cancelURL,
		IdempotencyKey: payment.OrderID.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("creating checkout session via %s: %w", in.Method, err)
	}

	method := in.Method
	payment.Status = domain.PaymentStatusCheckoutStarted
	payment.Gateway = &method
	payment.GatewayTransactionID = &result.GatewayTransactionID
	payment.CheckoutURL = &result.CheckoutURL
	payment.UpdatedAt = time.Now().UTC()

	if err := uc.repo.Update(ctx, payment); err != nil {
		return nil, fmt.Errorf("persisting checkout session for order %s: %w", in.OrderID, err)
	}

	succeeded = true
	return &StartCheckoutOutput{CheckoutURL: result.CheckoutURL}, nil
}
