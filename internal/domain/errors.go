package domain

import "errors"

var (
	ErrPaymentNotFound        = errors.New("payment not found")
	ErrAlreadyStaged          = errors.New("payment already staged for this order")
	ErrUnsupportedGateway     = errors.New("unsupported payment gateway")
	ErrInvalidWebhook         = errors.New("invalid webhook signature or payload")
	ErrForbidden              = errors.New("caller does not own this payment")
	ErrCheckoutInProgress     = errors.New("checkout already in progress for this order")
	ErrPaymentAlreadyResolved = errors.New("payment for this order has already been approved or rejected")
)
