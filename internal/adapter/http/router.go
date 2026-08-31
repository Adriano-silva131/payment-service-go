package httpadapter

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/adriano-linux/payment-service-go/internal/adapter/http/handler"
)

type RouterDeps struct {
	Checkout           *handler.CheckoutHandler
	StripeWebhook      *handler.WebhookHandler
	MercadoPagoWebhook *handler.WebhookHandler
	Health             *handler.HealthHandler
}

// NewRouter wires the HTTP surface described in the plan: checkout is behind the API
// Gateway's normal JWT auth, the two webhook routes are permit-all at the gateway (Stripe
// and Mercado Pago call them directly, with no app JWT), and health/metrics are
// unauthenticated infra endpoints, same as every other orderhub service.
func NewRouter(deps RouterDeps) http.Handler {
	r := chi.NewRouter()
	r.Use(Recoverer, otelhttp.NewMiddleware("payment-service"), RequestLogger)

	r.Get("/healthz", deps.Health.Liveness)
	r.Get("/readyz", deps.Health.Readiness)
	r.Handle("/metrics", promhttp.Handler())

	r.Route("/api/v1/payments", func(r chi.Router) {
		r.Post("/checkout", deps.Checkout.ServeHTTP)
		r.Post("/webhooks/stripe", deps.StripeWebhook.ServeHTTP)
		r.Post("/webhooks/mercadopago", deps.MercadoPagoWebhook.ServeHTTP)
	})

	return r
}
