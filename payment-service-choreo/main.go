package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// Event types (for structured logging)
const (
	EventServiceStart         = "service_start"
	EventRequestReceived      = "request_received"
	EventResponseSent         = "response_sent"
	EventSubscribed           = "event_subscribed"
	EventSubscriptionFailed   = "event_subscription_failed"
	EventOrderCreatedReceived = "order_created_event_received" // Used for logging event reception
	EventPaymentProcessed     = "payment_processed"
	EventPaymentFailed        = "payment_failed"
	EventInternalError        = "internal_error"
	EventPublishEventFailed   = "publish_event_failed"
)

// Common HTTP constants
const (
	ContentTypeHeader    = "Content-Type"
	ApplicationJSON      = "application/json"
	MethodNotAllowedMsg  = "Method not allowed"
	InvalidInputMsg      = "Invalid input"
	InternalServerErrMsg = "Internal server error"
)

// PaymentResponse Structs
type PaymentResponse struct {
	OrderID       string `json:"orderId"`
	TransactionID string `json:"transactionId"`
	Status        string `json:"status"`
	Message       string `json:"message"`
}

type EventBusPayload struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"` // Use RawMessage to defer decoding
}

type OrderCreatedEvent struct {
	OrderID string  `json:"orderId"`
	Amount  float64 `json:"amount"`
	Status  string  `json:"status"`
}

type PaymentSucceededEvent struct {
	OrderID         string  `json:"orderId"`
	TransactionID   string  `json:"transactionId"`
	Amount          float64 `json:"amount"`
	CustomerAddress string  `json:"customerAddress"`
}

type PaymentFailedEvent struct {
	OrderID string `json:"orderId"`
	Reason  string `json:"reason"`
}

// Global state for payments (in-memory for demo purposes)
var (
	payments     = make(map[string]PaymentResponse) // orderID -> PaymentResponse
	paymentMutex sync.RWMutex
	httpClient   *http.Client
)

// Global configuration variables, loaded from environment
var (
	eventBusURL              string
	paymentServiceChoreoPort string
	failPaymentThreshold     float64
)

func init() {
	// Initialize HTTP client with a timeout
	httpClient = &http.Client{
		Timeout: 10 * time.Second, // Global timeout for HTTP requests
	}

	// Load configuration from environment variables with defaults
	paymentServiceChoreoPort = getEnv("PAYMENT_SERVICE_CHOREO_PORT", "8082")
	eventBusURL = getEnv("EVENT_BUS_URL", "http://event-bus:8070")
	failPaymentThreshold = getEnvAsFloat("FAIL_PAYMENT_THRESHOLD", 150.00) // Default threshold
}

// Helper to get environment variables or use a default value
func getEnv(key, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return defaultValue
}

// Helper to get environment variables as float64
func getEnvAsFloat(key string, defaultValue float64) float64 {
	if valueStr, ok := os.LookupEnv(key); ok {
		if value, err := fmt.Sscanf(valueStr, "%f", &defaultValue); err == nil && value == 1 {
			return defaultValue
		}
		log.Printf("Warning: Invalid float value for %s: %s. Using default: %f", key, valueStr, defaultValue)
	}
	return defaultValue
}

// structuredLog logs messages in a structured (JSON) format
func structuredLog(eventType string, fields map[string]interface{}) {
	logEntry := make(map[string]interface{})
	logEntry["timestamp"] = time.Now().Format(time.RFC3339)
	logEntry["service"] = "payment-service-choreo"
	logEntry["event_type"] = eventType
	for k, v := range fields {
		logEntry[k] = v
	}
	jsonLog, err := json.Marshal(logEntry)
	if err != nil {
		log.Printf("error marshalling log entry: %v, original: %v", err, logEntry)
		return
	}
	fmt.Println(string(jsonLog))
}

// doWithRetry attempts to execute a function with exponential backoff.
func doWithRetry(ctx context.Context, operationName string, maxRetries int, initialDelay time.Duration, fn func() error) error {
	delay := initialDelay
	for i := 0; i < maxRetries; i++ {
		err := fn()
		if err == nil {
			return nil
		}
		structuredLog(EventInternalError, map[string]interface{}{
			"operation":    operationName,
			"attempt":      i + 1,
			"max_attempts": maxRetries,
			"error":        err.Error(),
			"retry_in":     delay.String(),
		})

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			delay *= 2 // Exponential backoff
		}
	}
	return fmt.Errorf("failed %s after %d retries: %w", operationName, maxRetries, ctx.Err())
}

// publishEvent sends an event to the Event Bus
func publishEvent(ctx context.Context, eventType string, data interface{}) error {
	payload := EventBusPayload{Type: eventType, Data: json.RawMessage(fmt.Sprintf("%s", data))}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal event payload: %w", err)
	}

	url := fmt.Sprintf("%s/publish", eventBusURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set(ContentTypeHeader, ApplicationJSON)

	return doWithRetry(ctx, "Publish Event to Event Bus", 3, 500*time.Millisecond, func() error {
		resp, clientErr := httpClient.Do(req)
		if clientErr != nil {
			return fmt.Errorf("http request failed: %w", clientErr)
		}
		defer func() { // Ensure the body is closed
			if closeErr := resp.Body.Close(); closeErr != nil {
				log.Printf("Warning: Error closing response body from Event Bus after publish: %v", closeErr)
			}
		}()
		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("event bus responded with status %d: %s", resp.StatusCode, string(bodyBytes))
		}
		return nil
	})
}

// subscribeToEventBus subscribes this service to a specific event type on the Event Bus.
func subscribeToEventBus(ctx context.Context, eventType, callbackURL string) {
	url := fmt.Sprintf("%s/subscribe", eventBusURL)
	subscription := map[string]string{
		"Type": eventType, // Use “Type” per Event Bus's SubscriberRequest
		"Url":  callbackURL,
	}
	jsonSubscription, err := json.Marshal(subscription)
	if err != nil {
		structuredLog(EventSubscriptionFailed, map[string]interface{}{
			"event_type": eventType,
			"callback":   callbackURL,
			"error":      err.Error(),
			"reason":     "marshal error",
		})
		return
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonSubscription))
	if err != nil {
		structuredLog(EventSubscriptionFailed, map[string]interface{}{
			"event_type": eventType,
			"callback":   callbackURL,
			"error":      err.Error(),
			"reason":     "create request error",
		})
		return
	}
	req.Header.Set(ContentTypeHeader, ApplicationJSON)

	err = doWithRetry(ctx, "Subscribe to Event Bus", 5, 1*time.Second, func() error {
		resp, clientErr := httpClient.Do(req)
		if clientErr != nil {
			return fmt.Errorf("http request failed: %w", clientErr)
		}
		defer func() { // Ensure the body is closed
			if closeErr := resp.Body.Close(); closeErr != nil {
				log.Printf("Warning: Error closing response body after subscribe: %v", closeErr)
			}
		}()
		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("event bus responded with status %d: %s", resp.StatusCode, string(bodyBytes))
		}
		return nil
	})

	if err != nil {
		structuredLog(EventSubscriptionFailed, map[string]interface{}{
			"event_type": eventType,
			"callback":   callbackURL,
			"error":      err.Error(),
			"reason":     "failed after retries",
		})
	} else {
		structuredLog(EventSubscribed, map[string]interface{}{
			"event_type": eventType,
			"callback":   callbackURL,
		})
	}
}

// handleSuccessfulPayment processes a successful payment and publishes the event.
func handleSuccessfulPayment(ctx context.Context, eventData OrderCreatedEvent, transactionID string) {
	paymentMutex.Lock()
	payments[eventData.OrderID] = PaymentResponse{
		OrderID:       eventData.OrderID,
		TransactionID: transactionID,
		Status:        "succeeded",
		Message:       "Payment processed successfully.",
	}
	paymentMutex.Unlock()

	structuredLog(EventPaymentProcessed, map[string]interface{}{
		"order_id":       eventData.OrderID,
		"transaction_id": transactionID,
		"status":         "succeeded",
		"amount":         eventData.Amount,
	})

	// Publish PaymentSucceeded event
	paymentSucceededEvent := PaymentSucceededEvent{
		OrderID:         eventData.OrderID,
		TransactionID:   transactionID,
		Amount:          eventData.Amount,
		CustomerAddress: "Simulated Address", // Not in original OrderCreated, adding for demo
	}
	eventDataBytes, err := json.Marshal(paymentSucceededEvent)
	if err != nil {
		structuredLog(EventInternalError, map[string]interface{}{
			"error":    err.Error(),
			"message":  "Failed to marshal PaymentSucceeded event data",
			"order_id": eventData.OrderID,
		})
		return
	}
	err = publishEvent(ctx, "PaymentSucceeded", string(eventDataBytes))
	if err != nil {
		structuredLog(EventPublishEventFailed, map[string]interface{}{
			"order_id":   eventData.OrderID,
			"event_type": "PaymentSucceeded",
			"error":      err.Error(),
		})
	}
}

// handleFailedPayment processes a failed payment and publishes the event.
func handleFailedPayment(ctx context.Context, eventData OrderCreatedEvent, transactionID string, reason string) {
	paymentMutex.Lock()
	payments[eventData.OrderID] = PaymentResponse{
		OrderID:       eventData.OrderID,
		TransactionID: transactionID,
		Status:        "failed",
		Message:       fmt.Sprintf("Payment failed due to %s.", reason),
	}
	paymentMutex.Unlock()

	structuredLog(EventPaymentFailed, map[string]interface{}{
		"order_id":       eventData.OrderID,
		"transaction_id": transactionID,
		"reason":         reason,
		"amount":         eventData.Amount,
	})

	// Publish PaymentFailed event
	paymentFailedEvent := PaymentFailedEvent{
		OrderID: eventData.OrderID,
		Reason:  reason,
	}
	eventDataBytes, err := json.Marshal(paymentFailedEvent)
	if err != nil {
		structuredLog(EventInternalError, map[string]interface{}{
			"error":    err.Error(),
			"message":  "Failed to marshal PaymentFailed event data",
			"order_id": eventData.OrderID,
		})
		return
	}
	err = publishEvent(ctx, "PaymentFailed", string(eventDataBytes))
	if err != nil {
		structuredLog(EventPublishEventFailed, map[string]interface{}{
			"order_id":   eventData.OrderID,
			"event_type": "PaymentFailed",
			"error":      err.Error(),
		})
	}
}

// simulatePaymentAndPublishEvents encapsulates the payment processing simulation and event publishing.
func simulatePaymentAndPublishEvents(eventData OrderCreatedEvent, transactionID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second) // New context for publishing
	defer cancel()

	time.Sleep(2 * time.Second) // Simulate work

	paymentMutex.Lock()
	defer paymentMutex.Unlock()

	currentPayment := payments[eventData.OrderID]
	if currentPayment.Status != "pending" {
		structuredLog(EventInternalError, map[string]interface{}{
			"order_id":       eventData.OrderID,
			"message":        "Payment already processed or cancelled, skipping simulation result.",
			"current_status": currentPayment.Status,
		})
		return
	}

	if eventData.Amount > failPaymentThreshold { // Simulate payment failure for high amounts
		handleFailedPayment(ctx, eventData, transactionID, "amount limit exceeded")
	} else {
		handleSuccessfulPayment(ctx, eventData, transactionID)
	}
}

// Handlers
func orderCreatedEventHandler(w http.ResponseWriter, r *http.Request) {
	structuredLog(EventRequestReceived, map[string]interface{}{
		"method":     r.Method,
		"path":       r.URL.Path,
		"from":       r.RemoteAddr,
		"event_type": "OrderCreated",
	})

	if r.Method != http.MethodPost {
		http.Error(w, MethodNotAllowedMsg, http.StatusMethodNotAllowed)
		return
	}

	var eventPayload EventBusPayload
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&eventPayload); err != nil {
		http.Error(w, InvalidInputMsg, http.StatusBadRequest)
		structuredLog(EventInternalError, map[string]interface{}{
			"error":   err.Error(),
			"message": "Failed to decode event bus payload",
		})
		return
	}
	defer func() { // Ensure the request body is closed
		if closeErr := r.Body.Close(); closeErr != nil {
			log.Printf("Warning: Error closing request body in orderCreatedEventHandler: %v", closeErr)
		}
	}()

	if eventPayload.Type != "OrderCreated" {
		http.Error(w, InvalidInputMsg, http.StatusBadRequest)
		structuredLog(EventInternalError, map[string]interface{}{
			"error":         "Mismatched event type",
			"expected_type": "OrderCreated",
			"received_type": eventPayload.Type,
		})
		return
	}

	var eventData OrderCreatedEvent
	if err := json.Unmarshal(eventPayload.Data, &eventData); err != nil {
		http.Error(w, InvalidInputMsg, http.StatusBadRequest)
		structuredLog(EventInternalError, map[string]interface{}{
			"error":    err.Error(),
			"message":  "Failed to unmarshal OrderCreated event data",
			"raw_data": string(eventPayload.Data),
		})
		return
	}

	transactionID := fmt.Sprintf("txn-%s-%d", eventData.OrderID, time.Now().UnixNano())

	paymentMutex.Lock()
	oldPayment, found := payments[eventData.OrderID]
	payments[eventData.OrderID] = PaymentResponse{
		OrderID:       eventData.OrderID,
		TransactionID: transactionID,
		Status:        "pending",
		Message:       "Processing payment...",
	}
	paymentMutex.Unlock()

	statusChangeFields := map[string]interface{}{
		"order_id":       eventData.OrderID,
		"old_status":     "none",
		"new_status":     "pending",
		"transaction_id": transactionID,
	}
	if found {
		statusChangeFields["old_status"] = oldPayment.Status
	}
	structuredLog(EventPaymentProcessed, statusChangeFields)

	// Simulate payment processing in a goroutine
	go simulatePaymentAndPublishEvents(eventData, transactionID)

	w.WriteHeader(http.StatusOK)
	structuredLog(EventResponseSent, map[string]interface{}{
		"status":   http.StatusOK,
		"order_id": eventData.OrderID,
		"message":  "OrderCreated event received, payment processing initiated.",
	})
}

// healthCheckHandler responds with 200 OK for health checks.
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, MethodNotAllowedMsg, http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	if _, err := io.WriteString(w, "OK"); err != nil {
		log.Printf("Warning: Error writing health check response: %v", err)
	}
}

func main() {
	http.HandleFunc("/pay/events/order_created", orderCreatedEventHandler)
	http.HandleFunc("/health", healthCheckHandler) // New health check endpoint

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe to OrderCreated event from Event Bus
	subscribeToEventBus(ctx, "OrderCreated", fmt.Sprintf("https://payment-service-choreo:%s/pay/events/order_created", paymentServiceChoreoPort))

	structuredLog(EventServiceStart, map[string]interface{}{"port": paymentServiceChoreoPort})
	if err := http.ListenAndServe(":"+paymentServiceChoreoPort, nil); err != nil {
		structuredLog(EventInternalError, map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
}
