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
	"strings" // Added for strings.TrimPrefix
	"sync"
	"time"
)

// Event types (for structured logging)
const (
	EventServiceStart                  = "service_start"
	EventRequestReceived               = "request_received"
	EventResponseSent                  = "response_sent"
	EventSubscribed                    = "event_subscribed"
	EventSubscriptionFailed            = "event_subscription_failed"
	EventOrderCreatedProcessed         = "order_created_event_processed"
	EventOrderStateUpdated             = "order_state_updated"
	EventInternalError                 = "internal_error"
	EventPaymentSucceededEventReceived = "payment_succeeded_event_received"
)

// Common HTTP constants
const (
	ContentTypeHeader    = "Content-Type"
	ApplicationJSON      = "application/json"
	MethodNotAllowedMsg  = "Method not allowed"
	InvalidInputMsg      = "Invalid input"
	InternalServerErrMsg = "Internal server error"
)

// OrderRequest Request/Response structs
type OrderRequest struct {
	OrderID string  `json:"orderId"`
	Amount  float64 `json:"amount"`
}

type OrderResponse struct {
	OrderID string `json:"orderId"`
	Status  string `json:"status"`
	Message string `json:"message"`
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

// Global state for orders (in-memory for demo purposes)
var (
	orders     = make(map[string]string) // orderID -> status
	orderMutex sync.RWMutex
	httpClient *http.Client
)

// Global configuration variables, loaded from environment
var (
	eventBusURL            string
	orderServiceChoreoPort string
)

func init() {
	// Initialize HTTP client with a timeout
	httpClient = &http.Client{
		Timeout: 10 * time.Second, // Global timeout for HTTP requests
	}

	// Load configuration from environment variables with defaults
	orderServiceChoreoPort = getEnv("ORDER_SERVICE_CHOREO_PORT", "8081")
	eventBusURL = getEnv("EVENT_BUS_URL", "http://event-bus:8070")
}

// Helper to get environment variables or use a default value
func getEnv(key, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return defaultValue
}

// structuredLog logs messages in a structured (JSON) format
func structuredLog(eventType string, fields map[string]interface{}) {
	logEntry := make(map[string]interface{})
	logEntry["timestamp"] = time.Now().Format(time.RFC3339)
	logEntry["service"] = "order-service-choreo"
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
func publishEvent(ctx context.Context, eventType string, data string) error { // Changed data type to string
	payload := EventBusPayload{Type: eventType, Data: json.RawMessage(data)} // Use data directly
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

// Handlers
func createOrderHandler(w http.ResponseWriter, r *http.Request) {
	structuredLog(EventRequestReceived, map[string]interface{}{
		"method": r.Method,
		"path":   r.URL.Path,
		"from":   r.RemoteAddr,
	})

	if r.Method != http.MethodPost {
		http.Error(w, MethodNotAllowedMsg, http.StatusMethodNotAllowed)
		structuredLog(EventResponseSent, map[string]interface{}{
			"status":  http.StatusMethodNotAllowed,
			"message": MethodNotAllowedMsg,
		})
		return
	}

	var req OrderRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, InvalidInputMsg, http.StatusBadRequest)
		structuredLog(EventResponseSent, map[string]interface{}{
			"status":  http.StatusBadRequest,
			"message": InvalidInputMsg,
			"error":   err.Error(),
		})
		return
	}
	defer func() { // Ensure the request body is closed
		if closeErr := r.Body.Close(); closeErr != nil {
			log.Printf("Warning: Error closing request body in createOrderHandler: %v", closeErr)
		}
	}()

	orderMutex.Lock()
	orders[req.OrderID] = "pending" // Initial status
	orderMutex.Unlock()

	structuredLog(EventOrderStateUpdated, map[string]interface{}{
		"order_id":   req.OrderID,
		"old_status": "none",
		"new_status": "pending",
	})

	// Publish OrderCreated event
	orderCreatedEvent := OrderCreatedEvent{
		OrderID: req.OrderID,
		Amount:  req.Amount,
		Status:  "pending",
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second) // Context for publishing
	defer cancel()

	// Marshal the specific event data to JSON string for the Data field.
	eventDataBytes, err := json.Marshal(orderCreatedEvent)
	if err != nil {
		structuredLog(EventInternalError, map[string]interface{}{
			"error":    err.Error(),
			"message":  "Failed to marshal OrderCreated event data",
			"order_id": req.OrderID,
		})
		http.Error(w, InternalServerErrMsg, http.StatusInternalServerError)
		return
	}

	err = publishEvent(ctx, "OrderCreated", string(eventDataBytes))
	if err != nil {
		structuredLog(EventInternalError, map[string]interface{}{
			"error":    err.Error(),
			"message":  "Failed to publish OrderCreated event",
			"order_id": req.OrderID,
		})
		http.Error(w, "Failed to publish OrderCreated event", http.StatusInternalServerError)
		return
	}

	resp := OrderResponse{
		OrderID: req.OrderID,
		Status:  "Order creation initiated (choreographed)",
		Message: "Order placed, awaiting payment confirmation via event.",
	}
	w.Header().Set(ContentTypeHeader, ApplicationJSON)
	if err := json.NewEncoder(w).Encode(resp); err != nil { // Handle Encode error
		log.Printf("Error encoding response for OrderID %s: %v", req.OrderID, err)
	}
	structuredLog(EventResponseSent, map[string]interface{}{
		"status":   http.StatusOK,
		"order_id": req.OrderID,
		"message":  resp.Message,
	})
}

// processOrderCreatedEvent handles the business logic for OrderCreated event.
func processOrderCreatedEvent(eventData OrderCreatedEvent) {
	orderMutex.Lock()
	oldStatus := orders[eventData.OrderID]
	orders[eventData.OrderID] = "processing payment"
	orderMutex.Unlock()

	structuredLog(EventOrderCreatedProcessed, map[string]interface{}{
		"order_id":   eventData.OrderID,
		"amount":     eventData.Amount,
		"old_status": oldStatus,
		"new_status": "processing payment",
	})
}

// processPaymentSucceededEvent handles the business logic for PaymentSucceeded event.
func processPaymentSucceededEvent(eventData PaymentSucceededEvent) {
	orderMutex.Lock()
	oldStatus := orders[eventData.OrderID]
	orders[eventData.OrderID] = "payment succeeded, shipping initiated"
	orderMutex.Unlock()

	structuredLog(EventPaymentSucceededEventReceived, map[string]interface{}{
		"order_id":         eventData.OrderID,
		"transaction_id":   eventData.TransactionID,
		"amount":           eventData.Amount,
		"customer_address": eventData.CustomerAddress,
		"old_status":       oldStatus,
		"new_status":       "payment succeeded, shipping initiated",
	})
}

// Common event handler logic
func handleIncomingEvent(w http.ResponseWriter, r *http.Request, expectedEventType string, processFunc func(json.RawMessage) error) {
	structuredLog(EventRequestReceived, map[string]interface{}{
		"method":     r.Method,
		"path":       r.URL.Path,
		"from":       r.RemoteAddr,
		"event_type": expectedEventType,
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
			log.Printf("Warning: Error closing request body in event handler: %v", closeErr)
		}
	}()

	if eventPayload.Type != expectedEventType {
		http.Error(w, InvalidInputMsg, http.StatusBadRequest)
		structuredLog(EventInternalError, map[string]interface{}{
			"error":         "Mismatched event type",
			"expected_type": expectedEventType,
			"received_type": eventPayload.Type,
		})
		return
	}

	if err := processFunc(eventPayload.Data); err != nil {
		http.Error(w, InvalidInputMsg, http.StatusBadRequest)
		structuredLog(EventInternalError, map[string]interface{}{
			"error":    err.Error(),
			"message":  "Failed to process event data",
			"raw_data": string(eventPayload.Data),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	structuredLog(EventResponseSent, map[string]interface{}{
		"status":  http.StatusOK,
		"message": fmt.Sprintf("%s event processed", expectedEventType),
	})
}

func orderCreatedEventHandler(w http.ResponseWriter, r *http.Request) {
	handleIncomingEvent(w, r, "OrderCreated", func(data json.RawMessage) error {
		var eventData OrderCreatedEvent
		if err := json.Unmarshal(data, &eventData); err != nil {
			return fmt.Errorf("failed to unmarshal OrderCreated event data: %w", err)
		}
		processOrderCreatedEvent(eventData)
		return nil
	})
}

func paymentSucceededEventHandler(w http.ResponseWriter, r *http.Request) {
	handleIncomingEvent(w, r, "PaymentSucceeded", func(data json.RawMessage) error {
		var eventData PaymentSucceededEvent
		if err := json.Unmarshal(data, &eventData); err != nil {
			return fmt.Errorf("failed to unmarshal PaymentSucceeded event data: %w", err)
		}
		processPaymentSucceededEvent(eventData)
		return nil
	})
}

// getOrderStatusHandler provides the current status of an order
func getOrderStatusHandler(w http.ResponseWriter, r *http.Request) {
	structuredLog(EventRequestReceived, map[string]interface{}{
		"method": r.Method,
		"path":   r.URL.Path,
		"from":   r.RemoteAddr,
	})

	if r.Method != http.MethodGet {
		http.Error(w, MethodNotAllowedMsg, http.StatusMethodNotAllowed)
		return
	}

	// Remove order ID from URL. Using strings.TrimPrefix for robustness.
	orderID := strings.TrimPrefix(r.URL.Path, "/orders/status/")

	orderMutex.RLock()
	status, found := orders[orderID]
	orderMutex.RUnlock()

	if !found {
		http.Error(w, "Order not found", http.StatusNotFound)
		structuredLog(EventResponseSent, map[string]interface{}{
			"status":   http.StatusNotFound,
			"order_id": orderID,
			"message":  "Order not found",
		})
		return
	}

	resp := map[string]string{
		"orderId": orderID,
		"status":  status,
	}

	w.Header().Set(ContentTypeHeader, ApplicationJSON)
	if err := json.NewEncoder(w).Encode(resp); err != nil { // Handle Encode error
		log.Printf("Error encoding response for OrderID %s: %v", orderID, err)
	}
	structuredLog(EventResponseSent, map[string]interface{}{
		"status":       http.StatusOK,
		"order_id":     orderID,
		"order_status": status,
		"message":      "Order status retrieved",
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
	http.HandleFunc("/orders", createOrderHandler)
	http.HandleFunc("/orders/events/order_created", orderCreatedEventHandler)
	http.HandleFunc("/orders/events/payment_succeeded", paymentSucceededEventHandler)
	http.HandleFunc("/orders/status/", getOrderStatusHandler) // New endpoint for status polling
	http.HandleFunc("/health", healthCheckHandler)            // New health check endpoint

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe to events from Event Bus
	// The callback URLs should be this service's internal URL (http, not https)
	subscribeToEventBus(ctx, "OrderCreated", fmt.Sprintf("https://order-service-choreo:%s/orders/events/order_created", orderServiceChoreoPort))
	subscribeToEventBus(ctx, "PaymentSucceeded", fmt.Sprintf("https://order-service-choreo:%s/orders/events/payment_succeeded", orderServiceChoreoPort))

	structuredLog(EventServiceStart, map[string]interface{}{"port": orderServiceChoreoPort})
	if err := http.ListenAndServe(":"+orderServiceChoreoPort, nil); err != nil {
		structuredLog(EventInternalError, map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
}
