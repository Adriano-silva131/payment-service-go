package event

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const OrderCreatedV1 = "order.created.v1"

type OrderCreatedEvent struct {
	OrderID       uuid.UUID       `json:"orderId"`
	OrderNumber   int64           `json:"orderNumber"`
	CustomerID    string          `json:"customerId"`
	CustomerEmail string          `json:"customerEmail"`
	TotalAmount   decimal.Decimal `json:"totalAmount"`
}
