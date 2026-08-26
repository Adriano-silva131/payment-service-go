package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adriano-linux/payment-service-go/internal/domain"
)

type PaymentRepository struct {
	pool *pgxpool.Pool
}

func NewPaymentRepository(pool *pgxpool.Pool) *PaymentRepository {
	return &PaymentRepository{pool: pool}
}

func (r *PaymentRepository) ExistsByOrderID(ctx context.Context, orderID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM payments WHERE order_id = $1)`, orderID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking payment existence: %w", err)
	}
	return exists, nil
}

func (r *PaymentRepository) Insert(ctx context.Context, p *domain.Payment) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO payments (id, order_id, customer_id, customer_email, amount, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, p.ID, p.OrderID, p.CustomerID, p.CustomerEmail, p.Amount, p.Status, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("inserting payment: %w", err)
	}
	return nil
}

func (r *PaymentRepository) FindByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.Payment, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, order_id, customer_id, customer_email, amount, status,
		       gateway, gateway_transaction_id, checkout_url, created_at, updated_at
		FROM payments WHERE order_id = $1
	`, orderID)
	return scanPayment(row)
}

func (r *PaymentRepository) FindByGatewayTransactionID(ctx context.Context, gateway domain.PaymentMethod, txID string) (*domain.Payment, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, order_id, customer_id, customer_email, amount, status,
		       gateway, gateway_transaction_id, checkout_url, created_at, updated_at
		FROM payments WHERE gateway = $1 AND gateway_transaction_id = $2
	`, gateway, txID)
	return scanPayment(row)
}

func (r *PaymentRepository) Update(ctx context.Context, p *domain.Payment) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE payments
		SET status = $1, gateway = $2, gateway_transaction_id = $3, checkout_url = $4, updated_at = $5
		WHERE id = $6
	`, p.Status, p.Gateway, p.GatewayTransactionID, p.CheckoutURL, p.UpdatedAt, p.ID)
	if err != nil {
		return fmt.Errorf("updating payment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrPaymentNotFound
	}
	return nil
}

func scanPayment(row pgx.Row) (*domain.Payment, error) {
	var p domain.Payment
	var gateway *domain.PaymentMethod
	var gatewayTxID, checkoutURL *string

	err := row.Scan(
		&p.ID, &p.OrderID, &p.CustomerID, &p.CustomerEmail, &p.Amount, &p.Status,
		&gateway, &gatewayTxID, &checkoutURL, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrPaymentNotFound
		}
		return nil, fmt.Errorf("scanning payment row: %w", err)
	}

	p.Gateway = gateway
	p.GatewayTransactionID = gatewayTxID
	p.CheckoutURL = checkoutURL
	return &p, nil
}
