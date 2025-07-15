#!/bin/bash

# Script per testare i pattern Saga (Orchestratore o Coreografo)

# Auxiliary functions
log_info() {
    echo "INFO: $1"
}

log_error() {
    echo "ERROR: $1" >&2
}

wait_for_service() {
    local service_name=$1
    local port=$2
    local host="localhost"
    local timeout=120
    log_info "Waiting for service '$service_name' to be available on $host:$port..."
    for _ in $(seq 1 $timeout); do
        if nc -z "$host" "$port" >/dev/null 2>&1; then
            log_info "Service '$service_name' available."
            return 0
        fi
        sleep 3
    done
    log_error "Timeout: The service '$service_name' is not available after $timeout seconds."
    return 1
}

# --- Start of script ---

if [ -z "$1" ]; then
    log_error "Usage: $0 <orchestrator|choreographer>"
    exit 1
fi

SAGA_TYPE=$1
ORDER_SERVICE_PORT=""
ORDER_SERVICE_NAME=""
ORDER_SERVICE_URL=""

if [ "$SAGA_TYPE" == "orchestrator" ]; then
    ORDER_SERVICE_PORT="8080"
    ORDER_SERVICE_NAME="orchestrator-order-service"
    ORDER_SERVICE_URL="http://localhost:8080/create_order"
    log_info "Test Saga: Orchestrator Pattern"
elif [ "$SAGA_TYPE" == "choreographer" ]; then
    ORDER_SERVICE_PORT="8082" # Mapped door for Choreographer Order Service
    ORDER_SERVICE_NAME="choreographer-order-service"
    ORDER_SERVICE_URL="http://localhost:8082/create_order"
    log_info "Test Saga: Choreographer Pattern"
else
    log_error "Invalid Saga Type: '$SAGA_TYPE'. Use 'orchestrator' or 'choreographer'."
    exit 1
fi

log_info "Starting Docker Compose services..."
# Start all services defined in the docker-compose.yml
# In a production environment, you may want to start only those services needed for the specific saga type
# but for local testing, starting everything is often easier.
docker-compose up -d --build

if [ $? -ne 0 ]; then
    log_error "Error while starting Docker Compose."
    docker-compose down
    exit 1
fi

# Wait until RabbitMQ is healthy
wait_for_service "RabbitMQ" 5672 || { docker-compose down; exit 1; }

# Wait for the Order service to be available
wait_for_service "$ORDER_SERVICE_NAME" "$ORDER_SERVICE_PORT" || { docker-compose down; exit 1; }

# Run tests
log_info "Sending the order creation request to $ORDER_SERVICE_URL..."

# Example order data
ORDER_DATA='{
    "product_id": "product-123",
    "quantity": 1,
    "customer_id": "customer-test"
}'

# Use a temporary file for curl verbose output
CURL_LOG_FILE=$(mktemp)

# Execute curl with -v (verbose) and redirect output to the temp file
# Use -w "%{http_code}" to get the HTTP status code even with -s
HTTP_STATUS=$(curl -v -X POST -H "Content-Type: application/json" -d "$ORDER_DATA" "$ORDER_SERVICE_URL" \
    --output /dev/null --write-out "%{http_code}" 2>"$CURL_LOG_FILE")

CURL_EXIT_CODE=$?

if [ "$CURL_EXIT_CODE" -ne 0 ]; then
    log_error "Error while sending cURL request (Exit Code: $CURL_EXIT_CODE)."
    log_error "cURL Verbose Output (from $CURL_LOG_FILE):"
    cat "$CURL_LOG_FILE" >&2 # Print the curl verbose output to stderr
    log_error "Dumping Docker Compose logs for choreographer services for analysis:"
    # Dump logs specifically for choreographer services (and main orchestrator for context)
    docker-compose logs --no-color choreographer-order-service choreographer-inventory-service choreographer-payment-service orchestrator_main >&2
    rm "$CURL_LOG_FILE" # Clean up temp file
    docker-compose down
    exit 1
fi

rm "$CURL_LOG_FILE" # Clean up temp file if curl was successful

log_info "Order service response HTTP Status: $HTTP_STATUS"

# You can add logic here to analyse the response and the final status of the order
# For example, extract the order_id from the JSON response and then check the service logs
# to see the outcome of the saga.

log_info "Test completed. You can check the Docker service logs for details:"
echo "docker-compose logs"

log_info "Wait a few seconds for the final propagation of events..."
sleep 5

log_info "Shutting down Docker Compose services..."
docker-compose down

log_info "Test script terminated."