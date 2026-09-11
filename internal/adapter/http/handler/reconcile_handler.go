package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/adriano-linux/payment-service-go/internal/domain"
	"github.com/adriano-linux/payment-service-go/internal/usecase"
)

type reconcileResponse struct {
	Status string `json:"status"`
}

type ReconcileHandler struct {
	reconcile *usecase.Reconcile
}

func NewReconcileHandler(reconcile *usecase.Reconcile) *ReconcileHandler {
	return &ReconcileHandler{reconcile: reconcile}
}

func (h *ReconcileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	orderID, err := uuid.Parse(chi.URLParam(r, "orderId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "orderId must be a valid UUID")
		return
	}

	customerID := r.Header.Get("X-User-Id")
	if customerID == "" {
		writeError(w, http.StatusBadRequest, "missing X-User-Id header")
		return
	}

	out, err := h.reconcile.Handle(r.Context(), usecase.ReconcileInput{OrderID: orderID, CustomerID: customerID})
	if err != nil {
		if errors.Is(err, domain.ErrPaymentNotFound) {
			writeError(w, http.StatusNotFound, "no payment found for this order")
			return
		}
		if errors.Is(err, domain.ErrForbidden) {
			writeError(w, http.StatusForbidden, "this order does not belong to you")
			return
		}
		if errors.Is(err, domain.ErrCheckoutNotStarted) {
			writeError(w, http.StatusConflict, "no checkout session has been started for this order yet")
			return
		}
		if errors.Is(err, domain.ErrPaymentStillPending) {
			writeJSON(w, http.StatusAccepted, map[string]string{"message": "the gateway has not resolved this payment yet"})
			return
		}
		slog.ErrorContext(r.Context(), "reconcile failed", "orderId", orderID, "error", err)
		writeError(w, http.StatusBadGateway, "failed to check payment status with gateway")
		return
	}

	writeJSON(w, http.StatusOK, reconcileResponse{Status: string(out.Status)})
}
