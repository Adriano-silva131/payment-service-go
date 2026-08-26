FROM golang:1.25-alpine AS build
WORKDIR /src

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/payment-service ./cmd/payment-service
RUN CGO_ENABLED=0 go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.19.1

FROM alpine:3.21
RUN apk add --no-cache ca-certificates wget

COPY --from=build /out/payment-service /usr/local/bin/payment-service
COPY --from=build /go/bin/migrate /usr/local/bin/migrate
COPY migrations /app/migrations
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

EXPOSE 8082
ENTRYPOINT ["docker-entrypoint.sh"]
