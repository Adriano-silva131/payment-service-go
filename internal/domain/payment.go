package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type PaymentStatus string

const (
	PaymentStatusPending         PaymentStatus = "PENDING"
	PaymentStatusCheckoutStarted PaymentStatus = "CHECKOUT_STARTED"
	PaymentStatusApproved        PaymentStatus = "APPROVED"
	PaymentStatusRejected        PaymentStatus = "REJECTED"
)

type PaymentMethod string

const (
	PaymentMethodStripe      PaymentMethod = "STRIPE"
	PaymentMethodMercadoPago PaymentMethod = "MERCADOPAGO"
)

type Payment struct {
	ID                   uuid.UUID
	OrderID              uuid.UUID
	OrderNumber          int64
	CustomerID           string
	CustomerEmail        string
	Amount               decimal.Decimal
	Status               PaymentStatus
	Gateway              *PaymentMethod
	GatewayTransactionID *string
	CheckoutURL          *string
	CheckoutAttempt      int64
	CreatedAt            time.Time
	UpdatedAt            time.Time
}
