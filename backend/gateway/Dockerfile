# FIRST STAGE: Builder
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY . .
RUN apk add --no-cache git

RUN go mod tidy
RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -o /api-gateway ./gateway/main.go

# --- SECOND STAGE: Light final image ---
FROM alpine:3.19

RUN adduser -D appuser
USER appuser

WORKDIR /app
COPY --from=builder /api-gateway ./api-gateway
EXPOSE 8000
CMD ["./api-gateway"]