package handler

import (
	"log/slog"
	"net/http"

	"github.com/adriano-linux/payment-service-go/internal/adapter/metrics"
	"github.com/adriano-linux/payment-service-go/internal/domain"
	"github.com/adriano-linux/payment-service-go/internal/usecase"
)

// WebhookHandler is shared by the /webhooks/stripe and /webhooks/mercadopago routes —
// the only difference between the two is which PaymentMethod is bound in main.go.
// Always responds 2xx once the webhook is authenticated (signature verified), even for
// event types the gateway adapter ignores, so the provider doesn't retry delivery
// forever; a bad signature is the only case that gets a non-2xx.
type WebhookHandler struct {
	handleWebhook *usecase.HandleWebhook
	method        domain.PaymentMethod
}

func NewWebhookHandler(handleWebhook *usecase.HandleWebhook, method domain.PaymentMethod) *WebhookHandler {
	return &WebhookHandler{handleWebhook: handleWebhook, method: method}
}

func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	err := h.handleWebhook.Handle(r.Context(), h.method, r)
	if err != nil {
		slog.ErrorContext(r.Context(), "webhook processing failed", "gateway", h.method, "error", err)
		metrics.WebhooksProcessed.WithLabelValues(string(h.method), "error").Inc()
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	metrics.WebhooksProcessed.WithLabelValues(string(h.method), "ok").Inc()
	w.WriteHeader(http.StatusOK)
}
