# FIRST STAGE: Builder
FROM golang:1.23.0-alpine AS builder

WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend /app/backend

WORKDIR /app/backend/orchestrator_saga/services/auth_service
RUN CGO_ENABLED=0 GOOS=linux go build -o /usr/local/bin/auth-service .

# --- SECOND STAGE: Light final image ---
FROM alpine:3.19

RUN adduser -D appuser
USER appuser

WORKDIR /home/appuser
COPY --from=builder /usr/local/bin/auth-service .
CMD ["./auth-service"]