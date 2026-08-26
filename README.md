# payment-service-go

Payment domain service for [OrderHub](https://github.com/Adriano-silva131/order-hub-application) — rewritten in Go with Clean Architecture, replacing the original Java `payment-service`. Owns the full payment lifecycle: staging a payment when an order is created, starting a real checkout session with **Stripe** (card) or **Mercado Pago** (pix/boleto), and resolving the result via gateway webhooks. Sandbox/test credentials only.

## Why Go, why a separate repo

The Java `payment-service` simulated payment approval with a hardcoded threshold (`amount > 10000 → REJECTED`, no network call). Replacing that with real Stripe/Mercado Pago integration was also the trigger to rewrite the service in Go — this repo is meant to stand on its own as a complete, idiomatic Go service (not a thin wrapper), which is also why it lives in its own repository rather than a subdirectory of the main monorepo.

## Architecture

Clean Architecture / ports & adapters:

```
cmd/payment-service   composition root — wiring only, no business logic
internal/domain       Payment, DltMessage — zero external imports
internal/usecase      StagePayment, StartCheckout, HandleWebhook + the ports
                       (PaymentRepository, EventPublisher, PaymentGateway) adapters implement
internal/adapter/
  http                chi router, checkout/webhook/health handlers
  kafka               order-events consumer, retry topic consumer, producer, DLT wiring
  postgres            pgx-based repositories
  gateway/stripe       Stripe adapter (Checkout Sessions)
  gateway/mercadopago  Mercado Pago adapter (Preferences)
  dlt                 scheduled dead-letter reprocessing
  metrics             Prometheus counters
migrations/           golang-migrate SQL files
```

`usecase` only depends on `domain` and the interfaces in `usecase/ports.go` — never on a concrete `adapter/*` package. Adapters implement those interfaces; `main.go` is the only place concrete adapters get wired together.

## Payment flow

1. `order.created.v1` (topic `order-events`) arrives → `StagePayment` idempotently inserts a `PENDING` row (same idempotency guarantee as the old Java `existsByOrderId` check).
2. Client calls `POST /api/v1/payments/checkout` with `{orderId, paymentMethod: "STRIPE"|"MERCADOPAGO"}`, authenticated via the API Gateway, which forwards the caller's identity as `X-User-Id` → `StartCheckout` rejects with 403 if that id doesn't match the payment's `CustomerID` (set from the trusted `order.created.v1` event, never from client input), otherwise resolves the gateway, creates a real checkout session, and returns a redirect URL.
3. The gateway calls back `POST /api/v1/payments/webhooks/stripe` or `/mercadopago` → the signature is verified, the payment is resolved to `APPROVED`/`REJECTED`, and `payment.processed.v1` is published to `payment-events` — same topic/event-type/shape the Java `order-service` and `notification-service` already consume, plus additive fields (`customerEmail`, `gateway`, `gatewayTransactionId`) that older consumers safely ignore.

No automatic fallback between gateways: Stripe and Mercado Pago cover different payment methods (card vs. pix/boleto), the client picks one explicitly.

## Retry + Dead Letter Table

Mirrors the Java `GenericEventConsumer`'s `@RetryableTopic` (4 attempts, 2s→4s→8s backoff, capped at 10s) and `@DltHandler`/`dlt_messages` table, without Spring Kafka's automatic per-attempt topic suffixing:

- One retry topic (`order-events-retry`) instead of N suffixed topics, carrying `x-retry-attempt` / `x-retry-not-before-ms` / `x-original-topic` headers.
- The main consumer never blocks on a failing message — it republishes to the retry topic and moves on.
- The retry consumer waits out each message's backoff window, retries up to 4 times total, then writes it to `dlt_messages` (same idempotency guard as the Java version: a partial unique index on `(original_topic, message_key)` where `status = 'PENDING'`).
- A goroutine reprocesses pending DLT rows every `DLT_REPROCESS_INTERVAL_MS` (default 60000, same as before), using `SELECT ... FOR UPDATE SKIP LOCKED` inside a transaction — the Postgres-native equivalent of the Java version's `@Lock(PESSIMISTIC_WRITE)`.

## Running locally

Requires the rest of the [order-hub-application](../order-hub-application) infra stack (Postgres, Kafka) running — this service reuses the existing `paymentdb` database.

```bash
cp .env.example .env   # fill in real Stripe/Mercado Pago sandbox credentials
export $(cat .env | xargs)
make migrate-up
make run
```

Via the main stack's `docker compose up -d --build` (from `order-hub-application/infra`), this repo is expected to be cloned as a **sibling directory** of `order-hub-application` — the compose file's build context points at `../../payment-service-go` by default (overridable via `PAYMENT_SERVICE_GO_PATH`).

### Testing sandbox webhooks locally

Stripe and Mercado Pago's sandboxes need a public URL to call back to. Point an ngrok tunnel at the API Gateway (not this service directly):

```bash
ngrok http 8000
```

Then register `https://<ngrok-id>.ngrok-free.app/api/v1/payments/webhooks/stripe` (and `/mercadopago`) in each provider's sandbox dashboard.

## Tests

```bash
make test              # unit tests (fakes, no external deps)
make test-integration  # testcontainers-go: real Postgres, runs this repo's own migrations
```

## Library choices

| Concern | Library | Why |
|---|---|---|
| HTTP router | `go-chi/chi/v5` | stdlib-compatible, minimal |
| Postgres | `jackc/pgx/v5` | de facto standard driver, better than `lib/pq` |
| Migrations | `golang-migrate/migrate/v4` | closest Go equivalent to Flyway |
| Kafka | `twmb/franz-go` | pure Go, no cgo/librdkafka |
| Metrics | `prometheus/client_golang` | official, `/metrics` mirrors `/actuator/prometheus` |
| Stripe | `stripe/stripe-go/v86` | official SDK, `webhook.ConstructEvent` handles signature verification |
| Mercado Pago | `mercadopago/sdk-go` | official SDK; webhook signature verification is hand-rolled HMAC-SHA256 per their docs (no SDK helper for it) |
| Config | `caarlos0/env/v11` | struct-tag env parsing, fails fast on missing required vars |
| Decimal | `shopspring/decimal` | avoids float precision issues for money, JSON-compatible with Java's `BigDecimal` |
| Tests | `stretchr/testify` + `testcontainers-go` | assertions + real-Postgres integration tests |

Repositories are hand-written `pgx` queries rather than `sqlc`-generated code — `sqlc` was the original plan for compile-time-checked SQL, but was dropped to avoid requiring a code-gen step/extra tool on every contributor's machine for a project this size. Worth revisiting if the schema grows.
