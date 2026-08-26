package usecase

import (
	"context"
	"errors"
	"fmt"

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

	if payment.Status != domain.PaymentStatusPending {
		return nil, fmt.Errorf("payment for order %s is not PENDING (status=%s), cannot start checkout", in.OrderID, payment.Status)
	}

	gw, err := uc.gateways.Resolve(in.Method)
	if err != nil {
		return nil, err
	}

	result, err := gw.CreateCheckout(ctx, CheckoutRequest{
		OrderID:       payment.OrderID,
		Amount:        payment.Amount,
		CustomerEmail: payment.CustomerEmail,
		SuccessURL:    uc.successURL,
		CancelURL:     uc.cancelURL,
	})
	if err != nil {
		return nil, fmt.Errorf("creating checkout session via %s: %w", in.Method, err)
	}

	method := in.Method
	payment.Gateway = &method
	payment.GatewayTransactionID = &result.GatewayTransactionID
	payment.CheckoutURL = &result.CheckoutURL

	if err := uc.repo.Update(ctx, payment); err != nil {
		return nil, fmt.Errorf("persisting checkout session for order %s: %w", in.OrderID, err)
	}

	return &StartCheckoutOutput{CheckoutURL: result.CheckoutURL}, nil
}
