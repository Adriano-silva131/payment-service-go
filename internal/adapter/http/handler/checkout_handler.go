package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/adriano-linux/payment-service-go/internal/adapter/metrics"
	"github.com/adriano-linux/payment-service-go/internal/domain"
	"github.com/adriano-linux/payment-service-go/internal/usecase"
)

type checkoutRequest struct {
	OrderID       string `json:"orderId" validate:"required,uuid"`
	PaymentMethod string `json:"paymentMethod" validate:"required,oneof=STRIPE MERCADOPAGO"`
}

type checkoutResponse struct {
	CheckoutURL string `json:"checkoutUrl"`
}

type CheckoutHandler struct {
	startCheckout *usecase.StartCheckout
	validate      *validator.Validate
}

func NewCheckoutHandler(startCheckout *usecase.StartCheckout) *CheckoutHandler {
	return &CheckoutHandler{startCheckout: startCheckout, validate: validator.New()}
}

func (h *CheckoutHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req checkoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.validate.Struct(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	orderID, err := uuid.Parse(req.OrderID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "orderId must be a valid UUID")
		return
	}

	customerID := r.Header.Get("X-User-Id")
	if customerID == "" {
		writeError(w, http.StatusBadRequest, "missing X-User-Id header")
		return
	}

	out, err := h.startCheckout.Handle(r.Context(), usecase.StartCheckoutInput{
		OrderID:    orderID,
		Method:     domain.PaymentMethod(req.PaymentMethod),
		CustomerID: customerID,
	})
	if err != nil {
		if errors.Is(err, domain.ErrPaymentNotFound) {
			writeError(w, http.StatusNotFound, "no pending payment found for this order")
			return
		}
		if errors.Is(err, domain.ErrForbidden) {
			writeError(w, http.StatusForbidden, "this order does not belong to you")
			return
		}
		slog.ErrorContext(r.Context(), "checkout failed", "orderId", orderID, "error", err)
		writeError(w, http.StatusBadGateway, "failed to start checkout with payment gateway")
		return
	}

	metrics.CheckoutsStarted.WithLabelValues(req.PaymentMethod).Inc()
	writeJSON(w, http.StatusOK, checkoutResponse{CheckoutURL: out.CheckoutURL})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
