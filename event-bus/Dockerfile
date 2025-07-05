FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o event-bus .

FROM alpine:3.22.0
WORKDIR /app
COPY --from=builder /app/event-bus .
EXPOSE 8070
CMD ["./event-bus"]