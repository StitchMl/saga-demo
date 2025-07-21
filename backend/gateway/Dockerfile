# FIRST STAGE: Builder
FROM golang:1.23.0-alpine AS builder

WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/gateway /app/gateway
COPY backend/common /app/common
RUN CGO_ENABLED=0 GOOS=linux go build -o /api-gateway /app/gateway/main.go
# --- SECOND STAGE: Light final image ---
FROM alpine:3.19

RUN adduser -D appuser
USER appuser

WORKDIR /home/appuser
COPY --from=builder /api-gateway .
EXPOSE 8000
CMD ["./api-gateway"]