package gateway_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/adriano-linux/payment-service-go/internal/adapter/gateway"
	"github.com/adriano-linux/payment-service-go/internal/domain"
	"github.com/adriano-linux/payment-service-go/internal/usecase"
)

type stubGateway struct {
	method domain.PaymentMethod
}

func (g *stubGateway) Method() domain.PaymentMethod { return g.method }

func (g *stubGateway) CreateCheckout(ctx context.Context, req usecase.CheckoutRequest) (*usecase.CheckoutResult, error) {
	return nil, nil
}

func (g *stubGateway) ParseWebhook(ctx context.Context, r *http.Request) (*usecase.WebhookNotification, error) {
	return nil, nil
}

func TestRegistry_ResolvesByMethod(t *testing.T) {
	stripe := &stubGateway{method: domain.PaymentMethodStripe}
	mercadoPago := &stubGateway{method: domain.PaymentMethodMercadoPago}
	registry := gateway.NewRegistry(stripe, mercadoPago)

	resolved, err := registry.Resolve(domain.PaymentMethodStripe)
	require.NoError(t, err)
	assert.Same(t, stripe, resolved)

	resolved, err = registry.Resolve(domain.PaymentMethodMercadoPago)
	require.NoError(t, err)
	assert.Same(t, mercadoPago, resolved)
}

func TestRegistry_UnknownMethodErrors(t *testing.T) {
	registry := gateway.NewRegistry(&stubGateway{method: domain.PaymentMethodStripe})

	_, err := registry.Resolve(domain.PaymentMethodMercadoPago)

	assert.ErrorIs(t, err, domain.ErrUnsupportedGateway)
}
