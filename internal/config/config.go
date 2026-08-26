package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	HTTPPort string `env:"HTTP_PORT" envDefault:"8082"`

	DatabaseURL string `env:"DATABASE_URL,required"`

	KafkaBrokers       []string `env:"KAFKA_BROKERS" envSeparator:"," envDefault:"localhost:9092"`
	KafkaConsumerGroup string   `env:"KAFKA_CONSUMER_GROUP_ID" envDefault:"payment-group"`

	DltReprocessIntervalMs int `env:"DLT_REPROCESS_INTERVAL_MS" envDefault:"60000"`

	StripeSecretKey     string `env:"STRIPE_SECRET_KEY"`
	StripeWebhookSecret string `env:"STRIPE_WEBHOOK_SECRET"`

	MercadoPagoAccessToken   string `env:"MERCADOPAGO_ACCESS_TOKEN"`
	MercadoPagoWebhookSecret string `env:"MERCADOPAGO_WEBHOOK_SECRET"`

	PaymentSuccessURL string `env:"PAYMENT_SUCCESS_URL" envDefault:"http://localhost:8000/checkout/success"`
	PaymentCancelURL  string `env:"PAYMENT_CANCEL_URL" envDefault:"http://localhost:8000/checkout/cancel"`
}

func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("loading config from environment: %w", err)
	}
	return cfg, nil
}
