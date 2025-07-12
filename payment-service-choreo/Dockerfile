FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix netgo -o payment-service-choreo .

FROM alpine:3.22.0
WORKDIR /app
RUN apk add --no-cache curl
COPY --from=builder /app/payment-service-choreo .
EXPOSE 8082
CMD ["./payment-service-choreo"]