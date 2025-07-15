FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY . .
RUN apk add --no-cache git

WORKDIR /app
RUN go mod tidy
RUN go mod download

WORKDIR /app/choreographer_saga/services/auth_service
RUN go build -o /usr/local/bin/auth-service .

FROM alpine:3.19

COPY --from=builder /usr/local/bin/auth-service /usr/local/bin/auth-service
CMD ["auth-service"]
EXPOSE 8090