package domain

import "errors"

var (
	ErrPaymentNotFound    = errors.New("payment not found")
	ErrAlreadyStaged      = errors.New("payment already staged for this order")
	ErrUnsupportedGateway = errors.New("unsupported payment gateway")
	ErrInvalidWebhook     = errors.New("invalid webhook signature or payload")
	ErrForbidden          = errors.New("caller does not own this payment")
)
