package stripe

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/shopspring/decimal"
	stripego "github.com/stripe/stripe-go/v86"
	"github.com/stripe/stripe-go/v86/webhook"

	"github.com/adriano-linux/payment-service-go/internal/domain"
	"github.com/adriano-linux/payment-service-go/internal/usecase"
)

const currency = "brl"

type Gateway struct {
	client        *stripego.Client
	webhookSecret string
}

func NewGateway(secretKey, webhookSecret string) *Gateway {
	return &Gateway{
		client:        stripego.NewClient(secretKey),
		webhookSecret: webhookSecret,
	}
}

func (g *Gateway) Method() domain.PaymentMethod {
	return domain.PaymentMethodStripe
}

func (g *Gateway) CreateCheckout(ctx context.Context, req usecase.CheckoutRequest) (*usecase.CheckoutResult, error) {
	amountInCents := req.Amount.Mul(decimal.NewFromInt(100)).Round(0).IntPart()

	params := &stripego.CheckoutSessionCreateParams{
		Mode:              stripego.String(string(stripego.CheckoutSessionModePayment)),
		SuccessURL:        stripego.String(req.SuccessURL + "?orderId=" + req.OrderID.String()),
		CancelURL:         stripego.String(req.CancelURL + "?orderId=" + req.OrderID.String()),
		CustomerEmail:     stripego.String(req.CustomerEmail),
		ClientReferenceID: stripego.String(req.OrderID.String()),
		LineItems: []*stripego.CheckoutSessionCreateLineItemParams{
			{
				Quantity: stripego.Int64(1),
				PriceData: &stripego.CheckoutSessionCreateLineItemPriceDataParams{
					Currency:   stripego.String(currency),
					UnitAmount: stripego.Int64(amountInCents),
					ProductData: &stripego.CheckoutSessionCreateLineItemPriceDataProductDataParams{
						Name: stripego.String(fmt.Sprintf("Pedido OrderHub %s", req.OrderID)),
					},
				},
			},
		},
	}

	if req.IdempotencyKey != "" {
		params.SetIdempotencyKey(req.IdempotencyKey)
	}

	session, err := g.client.V1CheckoutSessions.Create(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("creating stripe checkout session: %w", err)
	}

	return &usecase.CheckoutResult{
		GatewayTransactionID: session.ID,
		CheckoutURL:          session.URL,
	}, nil
}

func (g *Gateway) ParseWebhook(ctx context.Context, r *http.Request) (*usecase.WebhookNotification, error) {
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("reading stripe webhook body: %w", err)
	}
	sigHeader := r.Header.Get("Stripe-Signature")

	event, err := webhook.ConstructEvent(rawBody, sigHeader, g.webhookSecret)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrInvalidWebhook, err)
	}

	switch event.Type {
	case stripego.EventTypeCheckoutSessionCompleted, stripego.EventTypeCheckoutSessionAsyncPaymentSucceeded:
		var session stripego.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
			return nil, fmt.Errorf("unmarshalling checkout session from webhook: %w", err)
		}
		status := domain.PaymentStatusRejected
		if session.PaymentStatus == stripego.CheckoutSessionPaymentStatusPaid {
			status = domain.PaymentStatusApproved
		}
		return &usecase.WebhookNotification{GatewayTransactionID: session.ID, Status: status}, nil

	case stripego.EventTypeCheckoutSessionExpired, stripego.EventTypeCheckoutSessionAsyncPaymentFailed:
		var session stripego.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
			return nil, fmt.Errorf("unmarshalling checkout session from webhook: %w", err)
		}
		return &usecase.WebhookNotification{GatewayTransactionID: session.ID, Status: domain.PaymentStatusRejected}, nil

	default:
		return nil, nil
	}
}
