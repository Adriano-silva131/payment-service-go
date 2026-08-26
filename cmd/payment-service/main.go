package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/adriano-linux/payment-service-go/internal/adapter/dlt"
	"github.com/adriano-linux/payment-service-go/internal/adapter/gateway"
	"github.com/adriano-linux/payment-service-go/internal/adapter/gateway/mercadopago"
	"github.com/adriano-linux/payment-service-go/internal/adapter/gateway/stripe"
	httptransport "github.com/adriano-linux/payment-service-go/internal/adapter/http"
	"github.com/adriano-linux/payment-service-go/internal/adapter/http/handler"
	adapterkafka "github.com/adriano-linux/payment-service-go/internal/adapter/kafka"
	kafkahandler "github.com/adriano-linux/payment-service-go/internal/adapter/kafka/handler"
	"github.com/adriano-linux/payment-service-go/internal/adapter/postgres"
	"github.com/adriano-linux/payment-service-go/internal/config"
	"github.com/adriano-linux/payment-service-go/internal/domain"
	"github.com/adriano-linux/payment-service-go/internal/usecase"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	if err := run(); err != nil {
		slog.Error("fatal startup error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	paymentRepo := postgres.NewPaymentRepository(pool)
	dltRepo := postgres.NewDltRepository(pool)

	producerClient, err := kgo.NewClient(kgo.SeedBrokers(cfg.KafkaBrokers...))
	if err != nil {
		return err
	}
	defer producerClient.Close()
	producer := adapterkafka.NewProducer(producerClient)

	stripeGateway := stripe.NewGateway(cfg.StripeSecretKey, cfg.StripeWebhookSecret)
	mercadoPagoGateway, err := mercadopago.NewGateway(cfg.MercadoPagoAccessToken, cfg.MercadoPagoWebhookSecret)
	if err != nil {
		return err
	}
	gatewayRegistry := gateway.NewRegistry(stripeGateway, mercadoPagoGateway)

	stagePayment := usecase.NewStagePayment(paymentRepo)
	startCheckout := usecase.NewStartCheckout(paymentRepo, gatewayRegistry, cfg.PaymentSuccessURL, cfg.PaymentCancelURL)
	handleWebhook := usecase.NewHandleWebhook(paymentRepo, gatewayRegistry, producer)

	handlerRegistry := adapterkafka.NewHandlerRegistry(
		kafkahandler.NewOrderCreatedHandler(stagePayment),
	)

	mainConsumerClient, err := kgo.NewClient(
		kgo.SeedBrokers(cfg.KafkaBrokers...),
		kgo.ConsumerGroup(cfg.KafkaConsumerGroup),
		kgo.ConsumeTopics(adapterkafka.OrderEventsTopic),
		kgo.DisableAutoCommit(),
	)
	if err != nil {
		return err
	}
	defer mainConsumerClient.Close()

	retryTopic := adapterkafka.RetryTopic(adapterkafka.OrderEventsTopic)
	retryConsumerClient, err := kgo.NewClient(
		kgo.SeedBrokers(cfg.KafkaBrokers...),
		kgo.ConsumerGroup(cfg.KafkaConsumerGroup+"-retry"),
		kgo.ConsumeTopics(retryTopic),
		kgo.DisableAutoCommit(),
	)
	if err != nil {
		return err
	}
	defer retryConsumerClient.Close()

	mainConsumer := adapterkafka.NewConsumer(mainConsumerClient, adapterkafka.OrderEventsTopic, handlerRegistry, producer)
	retryConsumer := adapterkafka.NewRetryConsumer(retryConsumerClient, retryTopic, handlerRegistry, producer, dltRepo)
	reprocessor := dlt.NewReprocessor(dltRepo, producer)

	go mainConsumer.Run(ctx)
	go retryConsumer.Run(ctx)
	go reprocessor.Run(ctx, time.Duration(cfg.DltReprocessIntervalMs)*time.Millisecond)

	router := httptransport.NewRouter(httptransport.RouterDeps{
		Checkout:           handler.NewCheckoutHandler(startCheckout),
		StripeWebhook:      handler.NewWebhookHandler(handleWebhook, domain.PaymentMethodStripe),
		MercadoPagoWebhook: handler.NewWebhookHandler(handleWebhook, domain.PaymentMethodMercadoPago),
		Health:             handler.NewHealthHandler(pool),
	})

	server := &http.Server{
		Addr:    ":" + cfg.HTTPPort,
		Handler: router,
	}

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("http server listening", "port", cfg.HTTPPort)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	case err := <-serverErr:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}
