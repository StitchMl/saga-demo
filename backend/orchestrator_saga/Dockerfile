# FIRST STAGE: Builder
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN apk add --no-cache git
RUN go mod tidy
RUN go mod download
COPY backend /app/backend

WORKDIR /app/backend/orchestrator_saga
RUN CGO_ENABLED=0 GOOS=linux go build -o /usr/local/bin/orchestrator-app .

# --- SECOND STAGE: Light final image ---
FROM alpine:3.19

WORKDIR /root/
COPY --from=builder /usr/local/bin/orchestrator-app .
CMD ["./orchestrator-app"]