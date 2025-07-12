FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Compile the Go application for Linux, statically linked for Alpine
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix netgo -o event-bus .

FROM alpine:3.22.0
WORKDIR /app
# Install curl for healthchecks
RUN apk add --no-cache curl
# Copy the compiled executable from the stage builder
COPY --from=builder /app/event-bus .
EXPOSE 8070
CMD ["./event-bus"]