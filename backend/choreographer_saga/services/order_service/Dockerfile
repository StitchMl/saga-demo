# FIRST STAGE: Builder
FROM golang:1.23.0-alpine AS builder

WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend /app/backend

WORKDIR /app/backend/choreographer_saga/services/order_service
RUN CGO_ENABLED=0 GOOS=linux go build -o /usr/local/bin/order-service .

# --- SECOND STAGE: Light final image ---
FROM alpine:3.19

WORKDIR /root/
COPY --from=builder /usr/local/bin/order-service .
EXPOSE 8080
CMD ["./order-service"]