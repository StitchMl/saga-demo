#!/bin/bash

# Script to test the Saga patterns (Orchestrator or Choreographer)
# by interacting with the API Gateway.

# Fail fast
set -e

# --- Helper Functions ---
log_info() {
    echo "INFO: $1"
}

log_error() {
    echo "ERROR: $1" >&2
}

# --- Script Start ---

if [ "$#" -ne 2 ]; then
    log_error "Usage: $0 <orchestrated|choreographed> <success|fail_limit>"
    exit 1
fi

SAGA_TYPE=$1
TEST_CASE=$2
GATEWAY_URL="http://localhost:8000"
MAX_WAIT_SECONDS=60
POLL_INTERVAL_SECONDS=5

# Input validation
if [[ "$SAGA_TYPE" != "orchestrated" && "$SAGA_TYPE" != "choreographed" ]]; then
    log_error "Invalid Saga type: '$SAGA_TYPE'. Use 'orchestrated' or 'choreographed'."
    exit 1
fi

if [[ "$TEST_CASE" != "success" && "$TEST_CASE" != "fail_limit" ]]; then
    log_error "Invalid test case: '$TEST_CASE'. Use 'success' or 'fail_limit'."
    exit 1
fi

log_info "Starting services with Docker Compose..."
docker-compose up -d --build --force-recreate

log_info "Waiting for API Gateway to be healthy..."
SECONDS=0
while true; do
    if curl -sf "${GATEWAY_URL}/health" > /dev/null; then
        log_info "API Gateway is up and running."
        break
    fi
    if [ $SECONDS -ge $MAX_WAIT_SECONDS ]; then
        log_error "API Gateway did not become healthy within ${MAX_WAIT_SECONDS} seconds."
        docker-compose logs api-gateway
        docker-compose down
        exit 1
    fi
    sleep $POLL_INTERVAL_SECONDS
    SECONDS=$((SECONDS + POLL_INTERVAL_SECONDS))
    echo "Waiting... (${SECONDS}s / ${MAX_WAIT_SECONDS}s)"
done


log_info "--- Starting Test: Flow '$SAGA_TYPE', Case '$TEST_CASE' ---"

# 1. Register a new user for this test run to get a valid customer ID
UNIQUE_USER="tester_$(date +%s)"
log_info "Registering a new user '${UNIQUE_USER}' for flow '${SAGA_TYPE}'..."
REG_RESPONSE=$(curl -s -X POST \
    -H "Content-Type: application/json" \
    -d "{\"username\": \"${UNIQUE_USER}\", \"password\": \"password123\"}" \
    "${GATEWAY_URL}/register?flow=${SAGA_TYPE}")

CUSTOMER_ID=$(echo "$REG_RESPONSE" | grep -o '"customer_id":"[^"]*' | cut -d'"' -f4)

if [ -z "$CUSTOMER_ID" ]; then
    log_error "Failed to register user or parse customer_id from response: $REG_RESPONSE"
    docker-compose down
    exit 1
fi
log_info "User registered successfully. Customer ID: ${CUSTOMER_ID}"


# 2. Prepare order data based on the test case
# The payment limit is set to 2000.00 in docker-compose.yml
if [ "$TEST_CASE" == "success" ]; then
    log_info "Testing an order that should succeed (total < 2000.00)."
    ORDER_DATA='{"items": [{"product_id": "prod-1", "quantity": 1}]}' # Price: 199.9
else
    log_info "Testing an order that should fail due to payment limit (total > 2000.00)."
    ORDER_DATA='{"items": [{"product_id": "prod-1", "quantity": 11}]}' # Price: 2198.9
fi

# 3. Send the order creation request and validate the result
if [ "$TEST_CASE" == "success" ]; then
    MAX_RETRIES=5
    RETRY_COUNT=0
    SUCCESS=false

    while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
        log_info "Attempting to create order (Attempt $((RETRY_COUNT + 1))/${MAX_RETRIES})..."
        RESPONSE=$(curl -s -w "\nHTTP_STATUS:%{http_code}" -X POST \
            -H "Content-Type: application/json" \
            -H "X-Customer-ID: ${CUSTOMER_ID}" \
            -d "$ORDER_DATA" \
            "${GATEWAY_URL}/orders?flow=${SAGA_TYPE}")

        HTTP_BODY=$(echo "$RESPONSE" | sed '$d')
        HTTP_STATUS=$(echo "$RESPONSE" | tail -n1 | cut -d: -f2)

        log_info "Gateway Response -> Status: $HTTP_STATUS, Body: $HTTP_BODY"

        if [[ "$HTTP_STATUS" -ge 200 && "$HTTP_STATUS" -lt 300 ]]; then
            log_info "Initial request was accepted as expected."
            ORDER_ID=$(echo "$HTTP_BODY" | grep -o '"order_id":"[^"]*' | cut -d'"' -f4)

            # --- Differentiated Validation Logic ---
            if [ "$SAGA_TYPE" == "orchestrated" ]; then
                log_info "Validating synchronous response for orchestrated flow..."
                if echo "$HTTP_BODY" | grep -q '"status":"approved"'; then
                    log_info "TEST PASSED: Orchestrated saga completed synchronously with status 'approved'."
                    SUCCESS=true
                else
                    log_error "TEST FAILED: Orchestrated response did not contain 'status:approved'."
                    # This path is unlikely if status is 2xx, but it's a safeguard.
                fi
            else # This is the choreographed flow
                log_info "Polling logs for final status of order ${ORDER_ID}..."
                POLL_SECONDS=0
                while [ $POLL_SECONDS -lt $MAX_WAIT_SECONDS ]; do
                    if docker-compose logs --tail="50" | grep -q "Order ${ORDER_ID} status updated to approved"; then
                        log_info "TEST PASSED: Order was successfully approved."
                        SUCCESS=true
                        break
                    fi
                    sleep $POLL_INTERVAL_SECONDS
                    POLL_SECONDS=$((POLL_SECONDS + POLL_INTERVAL_SECONDS))
                done

                if ! $SUCCESS; then
                     log_error "TEST FAILED: Timed out waiting for order approval for ${ORDER_ID}."
                     docker-compose down
                     exit 1
                fi
            fi

            break # Exit retry loop
        elif [[ "$HTTP_STATUS" -eq 409 ]] && echo "$HTTP_BODY" | grep -q "Payment processing failed"; then
            log_info "Attempt failed due to a simulated random payment error. Retrying in 2 seconds..."
            RETRY_COUNT=$((RETRY_COUNT + 1))
            sleep 2
        else
            log_error "TEST FAILED: Expected a 2xx status, but got $HTTP_STATUS."
            docker-compose down
            exit 1
        fi
    done

    if ! $SUCCESS; then
        log_error "TEST FAILED: Exceeded max retries ($MAX_RETRIES) for the success case."
        docker-compose down
        exit 1
    fi

elif [ "$TEST_CASE" == "fail_limit" ]; then
    # This case is deterministic and doesn't need retries.
    log_info "Sending order creation request to ${GATEWAY_URL}/orders?flow=${SAGA_TYPE}..."
    RESPONSE=$(curl -s -w "\nHTTP_STATUS:%{http_code}" -X POST \
        -H "Content-Type: application/json" \
        -H "X-Customer-ID: ${CUSTOMER_ID}" \
        -d "$ORDER_DATA" \
        "${GATEWAY_URL}/orders?flow=${SAGA_TYPE}")

    HTTP_BODY=$(echo "$RESPONSE" | sed '$d')
    HTTP_STATUS=$(echo "$RESPONSE" | tail -n1 | cut -d: -f2)
    log_info "Gateway Response -> Status: $HTTP_STATUS, Body: $HTTP_BODY"

    # For both flows, the limit check is now synchronous
    if [[ "$HTTP_STATUS" -eq 400 || "$HTTP_STATUS" -eq 409 ]]; then
        log_info "TEST PASSED: Order was rejected immediately as expected."
    else
        log_error "TEST FAILED: Expected status 400 or 409, but got $HTTP_STATUS."
        docker-compose down
        exit 1
    fi
fi

log_info "--- Test Completed Successfully ---"

log_info "Relevant service logs:"
docker-compose logs --tail="20" api-gateway orchestrator choreographer-order-service choreographer-payment-service

log_info "Stopping Docker Compose services..."
docker-compose down

log_info "Test script finished."
