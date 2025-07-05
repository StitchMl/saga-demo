FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o payment-service .

FROM alpine:3.22.0
WORKDIR /app
COPY --from=builder /app/payment-service .
EXPOSE 8082
CMD ["./payment-service"]