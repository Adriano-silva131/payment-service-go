package usecase

import (
	"context"
	"log/slog"
	"time"

	"github.com/adriano-linux/payment-service-go/internal/domain"
)

type ReconcileSweep struct {
	repo       PaymentRepository
	gateways   GatewayResolver
	publisher  EventPublisher
	staleAfter time.Duration
}

func NewReconcileSweep(repo PaymentRepository, gateways GatewayResolver, publisher EventPublisher, staleAfter time.Duration) *ReconcileSweep {
	return &ReconcileSweep{repo: repo, gateways: gateways, publisher: publisher, staleAfter: staleAfter}
}

func (s *ReconcileSweep) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweepOnce(ctx)
		}
	}
}

func (s *ReconcileSweep) sweepOnce(ctx context.Context) {
	cutoff := time.Now().UTC().Add(-s.staleAfter)

	payments, err := s.repo.FindStaleCheckoutStarted(ctx, cutoff)
	if err != nil {
		slog.ErrorContext(ctx, "payment reconcile sweep: failed to list stale checkouts", "error", err)
		return
	}

	for _, payment := range payments {
		if err := s.reconcileOne(ctx, payment); err != nil {
			slog.ErrorContext(ctx, "payment reconcile sweep: failed to reconcile", "orderId", payment.OrderID, "error", err)
		}
	}
}

func (s *ReconcileSweep) reconcileOne(ctx context.Context, payment *domain.Payment) error {
	if payment.Gateway == nil || payment.GatewayTransactionID == nil {
		return nil
	}

	gw, err := s.gateways.Resolve(*payment.Gateway)
	if err != nil {
		return err
	}

	notification, err := gw.GetStatus(ctx, payment.OrderID, *payment.GatewayTransactionID)
	if err != nil {
		return err
	}
	if notification == nil {
		// Still genuinely unresolved on the gateway's side — nothing to do until it is,
		// or until this order gets picked up again on a later sweep.
		return nil
	}

	slog.InfoContext(ctx, "payment reconcile sweep: resolved stale checkout via gateway poll",
		"orderId", payment.OrderID, "status", notification.Status)
	return resolvePayment(ctx, s.repo, s.publisher, *payment.Gateway, notification)
}
