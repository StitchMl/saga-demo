# FIRST STAGE: Builder
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY . .
RUN apk add --no-cache git

WORKDIR /app
RUN go mod tidy
RUN go mod download

WORKDIR /app/backend/orchestrator_saga/services/payment_service
RUN CGO_ENABLED=0 GOOS=linux go build -o /usr/local/bin/payment-service .

# --- SECOND STAGE: Light final image ---
FROM alpine:3.19

WORKDIR /root/
COPY --from=builder /usr/local/bin/payment-service .
CMD ["./payment-service"]