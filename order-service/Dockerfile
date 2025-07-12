FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix netgo -o order-service .

# Stage 2: Final image
FROM alpine:3.22
WORKDIR /app
# QUESTA RIGA È FONDAMENTALE:
RUN apk add --no-cache curl
COPY --from=builder /app/order-service .
EXPOSE 8081
CMD ["./order-service"]