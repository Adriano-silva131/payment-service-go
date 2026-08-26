.PHONY: build test test-integration lint run migrate-up migrate-down

build:
	go build -o bin/payment-service ./cmd/payment-service

test:
	go test ./...

test-integration:
	go test -tags=integration ./test/integration/...

lint:
	go vet ./...
	gofmt -l .

run: build
	./bin/payment-service

migrate-up:
	migrate -path migrations -database "$$DATABASE_URL" up

migrate-down:
	migrate -path migrations -database "$$DATABASE_URL" down 1
