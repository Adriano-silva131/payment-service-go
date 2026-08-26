// Package event holds this service's own local copies of the Kafka event JSON shapes,
// matching orderhub's existing convention (order-service and payment-service each keep
// their own event record types rather than sharing a library).
package event

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const OrderCreatedV1 = "order.created.v1"

// OrderCreatedEvent mirrors order-service's OrderCreatedEvent.java record after the
// customerEmail field is added there. decimal.Decimal here accepts Jackson's bare-number
// BigDecimal encoding directly (no quotes needed).
type OrderCreatedEvent struct {
	OrderID       uuid.UUID       `json:"orderId"`
	CustomerID    string          `json:"customerId"`
	CustomerEmail string          `json:"customerEmail"`
	TotalAmount   decimal.Decimal `json:"totalAmount"`
}
