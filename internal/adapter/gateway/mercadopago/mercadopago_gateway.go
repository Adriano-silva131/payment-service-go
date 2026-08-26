package mercadopago

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	mpconfig "github.com/mercadopago/sdk-go/pkg/config"
	"github.com/mercadopago/sdk-go/pkg/payment"
	"github.com/mercadopago/sdk-go/pkg/preference"

	"github.com/adriano-linux/payment-service-go/internal/domain"
	"github.com/adriano-linux/payment-service-go/internal/usecase"
)

const currencyID = "BRL"

type Gateway struct {
	preferences   preference.Client
	payments      payment.Client
	webhookSecret string
}

func NewGateway(accessToken, webhookSecret string) (*Gateway, error) {
	cfg, err := mpconfig.New(accessToken)
	if err != nil {
		return nil, fmt.Errorf("configuring mercadopago sdk: %w", err)
	}

	return &Gateway{
		preferences:   preference.NewClient(cfg),
		payments:      payment.NewClient(cfg),
		webhookSecret: webhookSecret,
	}, nil
}

func (g *Gateway) Method() domain.PaymentMethod {
	return domain.PaymentMethodMercadoPago
}

func (g *Gateway) CreateCheckout(ctx context.Context, req usecase.CheckoutRequest) (*usecase.CheckoutResult, error) {
	amount, _ := req.Amount.Float64()

	pref, err := g.preferences.Create(ctx, preference.Request{
		ExternalReference: req.OrderID.String(),
		Items: []preference.ItemRequest{
			{
				Title:      fmt.Sprintf("Pedido OrderHub %s", req.OrderID),
				Quantity:   1,
				UnitPrice:  amount,
				CurrencyID: currencyID,
			},
		},
		Payer: &preference.PayerRequest{
			Email: req.CustomerEmail,
		},
		BackURLs: &preference.BackURLsRequest{
			Success: req.SuccessURL + "?orderId=" + req.OrderID.String(),
			Pending: req.SuccessURL + "?orderId=" + req.OrderID.String(),
			Failure: req.CancelURL + "?orderId=" + req.OrderID.String(),
		},
		AutoReturn: "approved",
	})
	if err != nil {
		return nil, fmt.Errorf("creating mercadopago preference: %w", err)
	}

	checkoutURL := pref.InitPoint
	if checkoutURL == "" {
		checkoutURL = pref.SandboxInitPoint
	}

	return &usecase.CheckoutResult{
		GatewayTransactionID: pref.ID,
		CheckoutURL:          checkoutURL,
	}, nil
}

func (g *Gateway) ParseWebhook(ctx context.Context, r *http.Request) (*usecase.WebhookNotification, error) {
	dataID := r.URL.Query().Get("data.id")
	if dataID == "" {
		return nil, fmt.Errorf("%w: missing data.id query parameter", domain.ErrInvalidWebhook)
	}

	if err := g.verifySignature(r, dataID); err != nil {
		return nil, err
	}

	notificationType := r.URL.Query().Get("type")
	if notificationType != "" && notificationType != "payment" {
		// Mercado Pago also sends notifications for other resource types
		// (merchant_order, etc.) that this integration doesn't act on.
		return nil, nil
	}

	paymentID, err := strconv.Atoi(dataID)
	if err != nil {
		return nil, fmt.Errorf("%w: non-numeric payment id %q", domain.ErrInvalidWebhook, dataID)
	}

	mpPayment, err := g.payments.Get(ctx, paymentID)
	if err != nil {
		return nil, fmt.Errorf("fetching mercadopago payment %d: %w", paymentID, err)
	}

	orderID, err := uuid.Parse(mpPayment.ExternalReference)
	if err != nil {
		return nil, fmt.Errorf("%w: payment %d has invalid external_reference %q", domain.ErrInvalidWebhook, paymentID, mpPayment.ExternalReference)
	}

	status := mapStatus(mpPayment.Status)
	if status == "" {
		// approved/rejected are terminal; pending/in_process/authorized etc. mean the
		// payment isn't resolved yet — no-op, wait for the next webhook delivery.
		return nil, nil
	}

	return &usecase.WebhookNotification{
		OrderID:              &orderID,
		GatewayTransactionID: dataID,
		Status:               status,
	}, nil
}

func mapStatus(mpStatus string) domain.PaymentStatus {
	switch mpStatus {
	case "approved":
		return domain.PaymentStatusApproved
	case "rejected", "cancelled":
		return domain.PaymentStatusRejected
	default:
		return ""
	}
}

func (g *Gateway) verifySignature(r *http.Request, dataID string) error {
	sigHeader := r.Header.Get("x-signature")
	requestID := r.Header.Get("x-request-id")
	if sigHeader == "" {
		return fmt.Errorf("%w: missing x-signature header", domain.ErrInvalidWebhook)
	}

	ts, v1, err := parseSignatureHeader(sigHeader)
	if err != nil {
		return err
	}

	manifest := fmt.Sprintf("id:%s;request-id:%s;ts:%s;", strings.ToLower(dataID), requestID, ts)

	mac := hmac.New(sha256.New, []byte(g.webhookSecret))
	mac.Write([]byte(manifest))
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expected), []byte(v1)) {
		return fmt.Errorf("%w: signature mismatch", domain.ErrInvalidWebhook)
	}
	return nil
}

func parseSignatureHeader(header string) (ts, v1 string, err error) {
	for _, part := range strings.Split(header, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "ts":
			ts = kv[1]
		case "v1":
			v1 = kv[1]
		}
	}
	if ts == "" || v1 == "" {
		return "", "", fmt.Errorf("%w: malformed x-signature header", domain.ErrInvalidWebhook)
	}
	return ts, v1, nil
}
