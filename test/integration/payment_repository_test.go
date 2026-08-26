//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	pg "github.com/adriano-linux/payment-service-go/internal/adapter/postgres"
	"github.com/adriano-linux/payment-service-go/internal/domain"
)

func TestPaymentRepository_RealPostgres(t *testing.T) {
	ctx := context.Background()

	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("paymentdb"),
		postgres.WithUsername("orderdb"),
		postgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategyAndDeadline(60*time.Second,
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2)),
		postgres.WithInitScripts(
			"../../migrations/000001_create_payments_table.up.sql",
			"../../migrations/000002_create_dlt_messages_table.up.sql",
			"../../migrations/000003_dlt_pending_unique_index.up.sql",
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pool, err := pg.NewPool(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	repo := pg.NewPaymentRepository(pool)
	orderID := uuid.New()

	exists, err := repo.ExistsByOrderID(ctx, orderID)
	require.NoError(t, err)
	require.False(t, exists)

	now := time.Now().UTC().Truncate(time.Microsecond)
	payment := &domain.Payment{
		ID:            uuid.New(),
		OrderID:       orderID,
		CustomerID:    "customer-1",
		CustomerEmail: "customer@example.com",
		Amount:        decimal.NewFromFloat(199.90),
		Status:        domain.PaymentStatusPending,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	require.NoError(t, repo.Insert(ctx, payment))

	exists, err = repo.ExistsByOrderID(ctx, orderID)
	require.NoError(t, err)
	require.True(t, exists)

	loaded, err := repo.FindByOrderID(ctx, orderID)
	require.NoError(t, err)
	require.Equal(t, domain.PaymentStatusPending, loaded.Status)
	require.Nil(t, loaded.Gateway)

	method := domain.PaymentMethodStripe
	txID := "cs_test_123"
	url := "https://checkout.stripe.com/cs_test_123"
	loaded.Gateway = &method
	loaded.GatewayTransactionID = &txID
	loaded.CheckoutURL = &url
	loaded.Status = domain.PaymentStatusApproved
	loaded.UpdatedAt = time.Now().UTC()
	require.NoError(t, repo.Update(ctx, loaded))

	byTx, err := repo.FindByGatewayTransactionID(ctx, domain.PaymentMethodStripe, "cs_test_123")
	require.NoError(t, err)
	require.Equal(t, domain.PaymentStatusApproved, byTx.Status)
	require.Equal(t, orderID, byTx.OrderID)
}
