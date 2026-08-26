CREATE TABLE payments (
    id                      UUID PRIMARY KEY,
    order_id                UUID NOT NULL UNIQUE,
    customer_id             VARCHAR(255) NOT NULL,
    customer_email          VARCHAR(255) NOT NULL,
    amount                  NUMERIC(19, 2) NOT NULL,
    status                  VARCHAR(50) NOT NULL,
    gateway                 VARCHAR(50),
    gateway_transaction_id  VARCHAR(255),
    checkout_url            TEXT,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_payments_gateway_tx
    ON payments (gateway, gateway_transaction_id)
    WHERE gateway_transaction_id IS NOT NULL;
