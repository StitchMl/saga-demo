FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix netgo -o shipping-service-choreo .

FROM alpine:3.22.0
WORKDIR /app
RUN apk add --no-cache curl
COPY --from=builder /app/shipping-service-choreo .
EXPOSE 8083
CMD ["./shipping-service-choreo"]