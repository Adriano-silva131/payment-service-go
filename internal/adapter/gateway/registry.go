package gateway

import (
	"fmt"

	"github.com/adriano-linux/payment-service-go/internal/domain"
	"github.com/adriano-linux/payment-service-go/internal/usecase"
)

type Registry struct {
	gateways map[domain.PaymentMethod]usecase.PaymentGateway
}

func NewRegistry(gateways ...usecase.PaymentGateway) *Registry {
	m := make(map[domain.PaymentMethod]usecase.PaymentGateway, len(gateways))
	for _, g := range gateways {
		m[g.Method()] = g
	}
	return &Registry{gateways: m}
}

func (r *Registry) Resolve(method domain.PaymentMethod) (usecase.PaymentGateway, error) {
	g, ok := r.gateways[method]
	if !ok {
		return nil, fmt.Errorf("%w: %s", domain.ErrUnsupportedGateway, method)
	}
	return g, nil
}
