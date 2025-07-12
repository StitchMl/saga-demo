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
	"strings"
	"sync"
	"time"
)

// Event types (for structured logging)
const (
	EventServiceStart             = "service_start"
	EventRequestReceived          = "request_received"
	EventResponseSent             = "response_sent"
	EventSubscribed               = "event_subscribed"
	EventSubscriptionFailed       = "event_subscription_failed"
	EventPaymentSucceededReceived = "payment_succeeded_event_received" // Used for logging event reception
	EventShippingProcessed        = "shipping_processed"
	EventShippingFailed           = "shipping_failed"
	EventInternalError            = "internal_error"
	EventPublishEventFailed       = "publish_event_failed"
)

// Common HTTP constants
const (
	ContentTypeHeader    = "Content-Type"
	ApplicationJSON      = "application/json"
	MethodNotAllowedMsg  = "Method not allowed"
	InvalidInputMsg      = "Invalid input"
	InternalServerErrMsg = "Internal server error"
)

// ShippingResponse Structs
type ShippingResponse struct {
	OrderID    string `json:"orderId"`
	TrackingID string `json:"trackingId"`
	Status     string `json:"status"`
	Message    string `json:"message"`
}

type EventBusPayload struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"` // Use RawMessage to defer decoding
}

type PaymentSucceededEvent struct {
	OrderID         string  `json:"orderId"`
	TransactionID   string  `json:"transactionId"`
	Amount          float64 `json:"amount"`
	CustomerAddress string  `json:"customerAddress"`
}

type ShippingSucceededEvent struct {
	OrderID    string `json:"orderId"`
	TrackingID string `json:"trackingId"`
}

type ShippingFailedEvent struct {
	OrderID string `json:"orderId"`
	Reason  string `json:"reason"`
}

// Global state for shipments (in-memory for demo purposes)
var (
	shipments     = make(map[string]ShippingResponse) // orderID → ShippingResponse
	shippingMutex sync.RWMutex
	httpClient    *http.Client
)

// Global configuration variables, loaded from environment
var (
	eventBusURL               string
	shippingServiceChoreoPort string
	failShippingOrderIDs      []string
)

func init() {
	// Initialize HTTP client with a timeout
	httpClient = &http.Client{
		Timeout: 10 * time.Second, // Global timeout for HTTP requests
	}

	// Load configuration from environment variables with defaults
	shippingServiceChoreoPort = getEnv("SHIPPING_SERVICE_CHOREO_PORT", "8083")
	eventBusURL = getEnv("EVENT_BUS_URL", "http://event-bus:8070")
	failShippingOrderIDs = getEnvAsSlice("FAIL_SHIPPING_ORDER_IDS", ",") // Comma-separated list of IDs
}

// Helper to get environment variables or use a default value
func getEnv(key, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return defaultValue
}

// Helper to get environment variables as a slice of strings
func getEnvAsSlice(key, separator string) []string {
	if valueStr, ok := os.LookupEnv(key); ok {
		if valueStr == "" {
			return []string{}
		}
		return splitAndTrim(valueStr, separator)
	}
	return []string{} // Default to empty slice
}

func splitAndTrim(s, sep string) []string {
	parts := make([]string, 0)
	for _, p := range strings.Split(s, sep) {
		trimmedP := strings.TrimSpace(p)
		if trimmedP != "" {
			parts = append(parts, trimmedP)
		}
	}
	return parts
}

// structuredLog logs messages in a structured (JSON) format
func structuredLog(eventType string, fields map[string]interface{}) {
	logEntry := make(map[string]interface{})
	logEntry["timestamp"] = time.Now().Format(time.RFC3339)
	logEntry["service"] = "shipping-service-choreo"
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

// extractAndValidatePaymentSucceededEvent extracts and validates the PaymentSucceededEvent from the request.
func extractAndValidatePaymentSucceededEvent(r *http.Request) (PaymentSucceededEvent, error) {
	var eventPayload EventBusPayload
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&eventPayload); err != nil {
		return PaymentSucceededEvent{}, fmt.Errorf("invalid event payload: %w", err)
	}

	if eventPayload.Type != "PaymentSucceeded" {
		return PaymentSucceededEvent{}, fmt.Errorf("mismatched event type: expected PaymentSucceeded, got %s", eventPayload.Type)
	}

	var eventData PaymentSucceededEvent
	if err := json.Unmarshal(eventPayload.Data, &eventData); err != nil {
		return PaymentSucceededEvent{}, fmt.Errorf("failed to decode PaymentSucceeded event data: %w", err)
	}
	return eventData, nil
}

// initiateShippingProcessing handles the actual shipping simulation and event publishing.
func initiateShippingProcessing(eventData PaymentSucceededEvent) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	time.Sleep(2 * time.Second) // Simulate work

	shippingMutex.Lock()
	defer shippingMutex.Unlock()

	currentShipping := shipments[eventData.OrderID]
	if currentShipping.Status != "pending" {
		structuredLog(EventInternalError, map[string]interface{}{
			"order_id":       eventData.OrderID,
			"message":        "Shipping already processed or cancelled, skipping simulation result.",
			"current_status": currentShipping.Status,
		})
		return
	}

	// Check if orderID is in the list of IDs to fail.
	shouldFail := false
	for _, id := range failShippingOrderIDs {
		if id == eventData.OrderID {
			shouldFail = true
			break
		}
	}

	trackingID := fmt.Sprintf("trk-%s-%d", eventData.OrderID, time.Now().UnixNano())

	if shouldFail {
		shipments[eventData.OrderID] = ShippingResponse{
			OrderID:    eventData.OrderID,
			TrackingID: trackingID,
			Status:     "failed",
			Message:    "Shipping failed due to simulation.",
		}
		structuredLog(EventShippingFailed, map[string]interface{}{
			"order_id":    eventData.OrderID,
			"tracking_id": trackingID,
			"reason":      "simulated failure",
		})

		// Publish ShippingFailed event
		shippingFailedEvent := ShippingFailedEvent{
			OrderID: eventData.OrderID,
			Reason:  "simulated failure",
		}
		eventDataBytes, err := json.Marshal(shippingFailedEvent)
		if err != nil {
			structuredLog(EventInternalError, map[string]interface{}{
				"error":    err.Error(),
				"message":  "Failed to marshal ShippingFailed event data",
				"order_id": eventData.OrderID,
			})
			return
		}
		err = publishEvent(ctx, "ShippingFailed", string(eventDataBytes))
		if err != nil {
			structuredLog(EventPublishEventFailed, map[string]interface{}{
				"order_id":   eventData.OrderID,
				"event_type": "ShippingFailed",
				"error":      err.Error(),
			})
		}
	} else {
		shipments[eventData.OrderID] = ShippingResponse{
			OrderID:    eventData.OrderID,
			TrackingID: trackingID,
			Status:     "succeeded",
			Message:    "Order shipped successfully.",
		}
		structuredLog(EventShippingProcessed, map[string]interface{}{
			"order_id":    eventData.OrderID,
			"tracking_id": trackingID,
			"status":      "succeeded",
		})

		// Publish ShippingSucceeded event
		shippingSucceededEvent := ShippingSucceededEvent{
			OrderID:    eventData.OrderID,
			TrackingID: trackingID,
		}
		eventDataBytes, err := json.Marshal(shippingSucceededEvent)
		if err != nil {
			structuredLog(EventInternalError, map[string]interface{}{
				"error":    err.Error(),
				"message":  "Failed to marshal ShippingSucceeded event data",
				"order_id": eventData.OrderID,
			})
			return
		}
		err = publishEvent(ctx, "ShippingSucceeded", string(eventDataBytes))
		if err != nil {
			structuredLog(EventPublishEventFailed, map[string]interface{}{
				"order_id":   eventData.OrderID,
				"event_type": "ShippingSucceeded",
				"error":      err.Error(),
			})
		}
	}
}

// Handlers
func paymentSucceededEventHandler(w http.ResponseWriter, r *http.Request) {
	structuredLog(EventRequestReceived, map[string]interface{}{
		"method":     r.Method,
		"path":       r.URL.Path,
		"from":       r.RemoteAddr,
		"event_type": "PaymentSucceeded",
	})

	if r.Method != http.MethodPost {
		http.Error(w, MethodNotAllowedMsg, http.StatusMethodNotAllowed)
		return
	}

	eventData, err := extractAndValidatePaymentSucceededEvent(r)
	if err != nil {
		http.Error(w, InvalidInputMsg, http.StatusBadRequest)
		structuredLog(EventInternalError, map[string]interface{}{
			"error":   err.Error(),
			"message": "Failed to extract and validate PaymentSucceeded event",
		})
		return
	}
	defer func() { // Ensure the request body is closed
		if closeErr := r.Body.Close(); closeErr != nil {
			log.Printf("Warning: Error closing request body in paymentSucceededEventHandler: %v", closeErr)
		}
	}()

	trackingID := fmt.Sprintf("trk-%s-%d", eventData.OrderID, time.Now().UnixNano())

	shippingMutex.Lock()
	oldShipping, found := shipments[eventData.OrderID]
	shipments[eventData.OrderID] = ShippingResponse{
		OrderID:    eventData.OrderID,
		TrackingID: trackingID,
		Status:     "pending",
		Message:    "Processing shipment...",
	}
	shippingMutex.Unlock()

	statusChangeFields := map[string]interface{}{
		"order_id":    eventData.OrderID,
		"old_status":  "none",
		"new_status":  "pending",
		"tracking_id": trackingID,
	}
	if found {
		statusChangeFields["old_status"] = oldShipping.Status
	}
	structuredLog(EventShippingProcessed, statusChangeFields)

	// Simulate shipping processing in a goroutine
	go initiateShippingProcessing(eventData)

	w.WriteHeader(http.StatusOK)
	structuredLog(EventResponseSent, map[string]interface{}{
		"status":   http.StatusOK,
		"order_id": eventData.OrderID,
		"message":  "PaymentSucceeded event received, shipping processing initiated.",
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
	http.HandleFunc("/ship/events/payment_succeeded", paymentSucceededEventHandler)
	http.HandleFunc("/health", healthCheckHandler) // New health check endpoint

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe to PaymentSucceeded event from Event Bus
	subscribeToEventBus(ctx, "PaymentSucceeded", fmt.Sprintf("https://shipping-service-choreo:%s/ship/events/payment_succeeded", shippingServiceChoreoPort))

	structuredLog(EventServiceStart, map[string]interface{}{"port": shippingServiceChoreoPort})
	if err := http.ListenAndServe(":"+shippingServiceChoreoPort, nil); err != nil {
		structuredLog(EventInternalError, map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
}
