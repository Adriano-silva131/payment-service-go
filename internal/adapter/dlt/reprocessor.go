package dlt

import (
	"context"
	"log/slog"
	"time"

	"github.com/adriano-linux/payment-service-go/internal/domain"
	"github.com/adriano-linux/payment-service-go/internal/usecase"
)

type republisher interface {
	PublishWithHeaders(ctx context.Context, topic, key string, headers map[string]string, payload []byte) error
}

type Reprocessor struct {
	dltRepo   usecase.DltRepository
	publisher republisher
	batchSize int
}

func NewReprocessor(dltRepo usecase.DltRepository, publisher republisher) *Reprocessor {
	return &Reprocessor{dltRepo: dltRepo, publisher: publisher, batchSize: 100}
}

func (r *Reprocessor) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.reprocessOnce(ctx)
		}
	}
}

func (r *Reprocessor) reprocessOnce(ctx context.Context) {
	err := r.dltRepo.ProcessPendingLocked(ctx, r.batchSize, func(ctx context.Context, msg *domain.DltMessage) error {
		headers := map[string]string{"event-type": msg.EventType}
		if err := r.publisher.PublishWithHeaders(ctx, msg.OriginalTopic, msg.MessageKey, headers, []byte(msg.Payload)); err != nil {
			slog.ErrorContext(ctx, "dlt reprocess: republish failed, leaving PENDING",
				"topic", msg.OriginalTopic, "id", msg.ID, "error", err)
			return err
		}
		slog.InfoContext(ctx, "dlt reprocess: republished", "topic", msg.OriginalTopic, "id", msg.ID)
		return nil
	})
	if err != nil {
		slog.ErrorContext(ctx, "dlt reprocess tick failed", "error", err)
	}
}
