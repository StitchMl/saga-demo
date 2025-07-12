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
	"time"
)

// Saga definitions
const (
	CreateOrderStep    = "create_order"
	ProcessPaymentStep = "process_payment"
	ShipOrderStep      = "ship_order"

	CancelOrderCompensation   = "cancel_order"
	RefundPaymentCompensation = "refund_payment"

	// Constants for compensation path suffixes
	cancelPathSuffix = "/cancel/%s" // For example /orders/cancel/{orderId}
	refundPathSuffix = "/refund/%s" // For example /pay/refund/{orderId}
)

// Event types (for structured logging)
const (
	EventSagaStarted             = "saga_started"
	EventSagaStepExecuted        = "saga_step_executed"
	EventSagaStepFailed          = "saga_step_failed"
	EventSagaCompensationStarted = "saga_compensation_started"
	EventSagaCompensationFailed  = "saga_compensation_failed"
	EventSagaCompleted           = "saga_completed"
	EventSagaFailed              = "saga_failed"
	EventRequestReceived         = "request_received"
	EventResponseSent            = "response_sent"
)

// Common HTTP constants
const (
	ContentTypeHeader    = "Content-Type"
	ApplicationJSON      = "application/json"
	MethodNotAllowedMsg  = "Method not allowed"
	InvalidInputMsg      = "Invalid input"
	InternalServerErrMsg = "Internal server error"
)

// OrderRequest Struct (still needed for incoming requests from a frontend)
type OrderRequest struct {
	OrderID string  `json:"orderId"` // This will be the initial ID from a frontend, empty.
	Amount  float64 `json:"amount"`
}

// OrderResponse Struct for parsing the response from order-service
type OrderResponse struct {
	OrderID string `json:"orderId"` // The actual generated OrderID from order-service
	Status  string `json:"status"`
	Message string `json:"message"`
}

// Global configuration variables, loaded from environment
var (
	orderServiceURL        string
	paymentServiceURL      string
	shippingServiceURL     string
	defaultCustomerAddress string // Used for simulation
	orchestratorPort       string
	httpClient             *http.Client
)

func init() {
	// Initialize HTTP client with a timeout
	httpClient = &http.Client{
		Timeout: 10 * time.Second, // Global timeout for HTTP requests
	}

	// Load configuration from environment variables with defaults
	orchestratorPort = getEnv("ORCHESTRATOR_PORT", "8080")
	orderServiceURL = getEnv("ORDER_SERVICE_URL", "http://order-service:8081/orders")
	paymentServiceURL = getEnv("PAYMENT_SERVICE_URL", "http://payment-service:8082/pay")
	shippingServiceURL = getEnv("SHIPPING_SERVICE_URL", "http://shipping-service:8083/ship")
	defaultCustomerAddress = getEnv("DEFAULT_CUSTOMER_ADDRESS", "Via Roma 1, Milano") // Default for simulation
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
	logEntry["service"] = "orchestrator"
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
		structuredLog(EventSagaStepFailed, map[string]interface{}{
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

// prepareRequest handles marshaling data and creating the http.Request.
func prepareRequest(ctx context.Context, url string, data interface{}) (*http.Request, error) {
	var jsonBody []byte
	var err error
	if data != nil { // Only marshal if data is provided
		jsonBody, err = json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
	} else {
		jsonBody = []byte{} // Empty body for nil data
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set(ContentTypeHeader, ApplicationJSON)
	return req, nil
}

// performSingleHttpRequest executes a single HTTP request and checks its status.
// It returns the response for further processing (reading body), or an error if the request failed
// or returned a non-2xx status. It closes the body only for non-2xx status to read the error message.
func performSingleHttpRequest(req *http.Request) (*http.Response, error) {
	resp, clientErr := httpClient.Do(req)
	if clientErr != nil {
		return nil, fmt.Errorf("http request failed: %w", clientErr)
	}
	// Check status code immediately after successful request
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body) // Read body for error message
		// Ensure the body is closed after reading.
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("Warning: Error closing response body after non-2xx status: %v", closeErr)
		}
		return nil, fmt.Errorf("service responded with status %d: %s", resp.StatusCode, string(bodyBytes))
	}
	return resp, nil // Return resp for the caller to read and close the body.
}

// callService makes an HTTP POST request to a given URL with data and returns the response body.
func callService(ctx context.Context, url string, data interface{}) ([]byte, error) {
	req, err := prepareRequest(ctx, url, data)
	if err != nil {
		return nil, err
	}

	var resp *http.Response
	retryErr := doWithRetry(ctx, "HTTP Call to "+url, 3, 500*time.Millisecond, func() error {
		var singleAttemptErr error
		resp, singleAttemptErr = performSingleHttpRequest(req)
		if singleAttemptErr != nil {
			return singleAttemptErr
		}
		return nil
	})

	if retryErr != nil {
		// If retry failed, ensure the last response body is closed if it exists.
		if resp != nil && resp.Body != nil {
			if closeErr := resp.Body.Close(); closeErr != nil {
				log.Printf("Warning: Error closing response body after retry failure: %v", closeErr)
			}
		}
		return nil, retryErr
	}

	// Read and close body for successful response
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		// Ensure the body is closed even if reading fails
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("Warning: Error closing response body after read failure: %v", closeErr)
		}
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	// Explicitly close body after a successful read
	if closeErr := resp.Body.Close(); closeErr != nil {
		log.Printf("Warning: Error closing response body after successful read: %v", closeErr)
	}

	return bodyBytes, nil
}

// Saga execution logic
func executeSaga(ctx context.Context, initialOrderID string, amount float64) string {
	// actualOrderID will store the ID generated by the order-service
	var actualOrderID string

	structuredLog(EventSagaStarted, map[string]interface{}{
		"initial_order_id": initialOrderID, // Log the ID from the frontend for context
		"amount":           amount,
	})

	// Step 1: Create Order
	// We send an empty OrderID in the request, as order-service generates its own UUID.
	orderCreateReq := struct {
		Amount float64 `json:"amount"`
	}{
		Amount: amount,
	}
	structuredLog(EventSagaStepExecuted, map[string]interface{}{
		"step": CreateOrderStep,
		"url":  orderServiceURL,
	})
	orderRespBytes, err := callService(ctx, orderServiceURL, orderCreateReq) // Capture the response body
	if err != nil {
		structuredLog(EventSagaStepFailed, map[string]interface{}{
			"step":     CreateOrderStep,
			"order_id": initialOrderID, // Use initial ID for error log if actual not available
			"error":    err.Error(),
		})
		structuredLog(EventSagaFailed, map[string]interface{}{
			"order_id": initialOrderID,
			"reason":   "Order creation failed",
		})
		return fmt.Sprintf("SAGA Failed: %v", err)
	}

	// Parse the response from order-service to get the actual generated OrderID.
	var createdOrder OrderResponse // Use the OrderResponse struct to unmarshal
	if err := json.Unmarshal(orderRespBytes, &createdOrder); err != nil {
		structuredLog(EventSagaFailed, map[string]interface{}{
			"reason":        "Failed to parse order service response",
			"error":         err.Error(),
			"response_body": string(orderRespBytes),
		})
		return fmt.Sprintf("SAGA Failed: Failed to parse order service response: %v", err)
	}
	actualOrderID = createdOrder.OrderID // This is the crucial assignment.

	// If order-service didn't return an ID, it is an unexpected scenario for this design.
	if actualOrderID == "" {
		structuredLog(EventSagaFailed, map[string]interface{}{
			"reason":        "Order service returned empty OrderID",
			"response_body": string(orderRespBytes),
		})
		return "SAGA Failed: Order service returned empty OrderID."
	}

	structuredLog(EventSagaStepExecuted, map[string]interface{}{
		"step":     CreateOrderStep,
		"order_id": actualOrderID, // Now logging the actual generated ID
		"message":  "Order created successfully by order-service",
	})

	// Step 2: Process Payment
	paymentReq := struct {
		OrderID         string  `json:"orderId"`
		Amount          float64 `json:"amount"`
		CustomerAddress string  `json:"customerAddress"`
	}{
		OrderID:         actualOrderID, // Use the actual generated OrderID
		Amount:          amount,
		CustomerAddress: defaultCustomerAddress, // Using configurable default
	}
	structuredLog(EventSagaStepExecuted, map[string]interface{}{
		"step":     ProcessPaymentStep,
		"order_id": actualOrderID,
		"url":      paymentServiceURL,
	})
	_, err = callService(ctx, paymentServiceURL, paymentReq)
	if err != nil {
		structuredLog(EventSagaStepFailed, map[string]interface{}{
			"step":     ProcessPaymentStep,
			"order_id": actualOrderID,
			"error":    err.Error(),
		})
		// Compensation: Cancel Order
		structuredLog(EventSagaCompensationStarted, map[string]interface{}{
			"compensation_for": ProcessPaymentStep,
			"compensation":     CancelOrderCompensation,
			"order_id":         actualOrderID,
		})
		// For compensation, the order-service expects the ID in the path.
		// Order-service for a cancel might ignore or not need the body.
		// If order-service expects a body for a cancel, uncomment and adjust `cancelOrderReq`.
		// cancelOrderReq: = struct { OrderID string `json:"orderId"`}{ OrderID: actualOrderID, }
		_, compErr := callService(ctx, fmt.Sprintf(orderServiceURL+cancelPathSuffix, actualOrderID), nil) // Pass nil, or a specific body if needed
		if compErr != nil {
			structuredLog(EventSagaCompensationFailed, map[string]interface{}{
				"compensation":   CancelOrderCompensation,
				"order_id":       actualOrderID,
				"error":          compErr.Error(),
				"original_error": err.Error(),
			})
			structuredLog(EventSagaFailed, map[string]interface{}{
				"order_id": actualOrderID,
				"reason":   "Payment failed and order compensation failed",
			})
			return fmt.Sprintf("SAGA Failed: Payment failed (%v) and order compensation also failed (%v)", err, compErr)
		}
		structuredLog(EventSagaFailed, map[string]interface{}{
			"order_id": actualOrderID,
			"reason":   "Payment failed, order compensated",
		})
		return fmt.Sprintf("SAGA Failed: Payment failed (%v), order cancelled.", err)
	}

	// Step 3: Ship Order
	shippingReq := struct {
		OrderID         string `json:"orderId"`
		CustomerAddress string `json:"customerAddress"`
	}{
		OrderID:         actualOrderID,          // Use the actual generated OrderID
		CustomerAddress: defaultCustomerAddress, // Using configurable default
	}
	structuredLog(EventSagaStepExecuted, map[string]interface{}{
		"step":     ShipOrderStep,
		"order_id": actualOrderID,
		"url":      shippingServiceURL,
	})
	_, err = callService(ctx, shippingServiceURL, shippingReq)
	if err != nil {
		structuredLog(EventSagaStepFailed, map[string]interface{}{
			"step":     ShipOrderStep,
			"order_id": actualOrderID,
			"error":    err.Error(),
		})
		// Compensation 1: Refund Payment
		structuredLog(EventSagaCompensationStarted, map[string]interface{}{
			"compensation_for": ShipOrderStep,
			"compensation":     RefundPaymentCompensation,
			"order_id":         actualOrderID,
		})
		// The payment-service expects the ID in the path for /pay/refund/{orderId}
		// If payment-service expects a body for refund, uncomment and adjust `refundPaymentReq`.
		// refundPaymentReq: = struct { OrderID string `json:"orderId"`}{ OrderID: actualOrderID, }
		_, compErr1 := callService(ctx, fmt.Sprintf(paymentServiceURL+refundPathSuffix, actualOrderID), nil) // Pass nil or specific body
		if compErr1 != nil {
			structuredLog(EventSagaCompensationFailed, map[string]interface{}{
				"compensation":   RefundPaymentCompensation,
				"order_id":       actualOrderID,
				"error":          compErr1.Error(),
				"original_error": err.Error(),
			})
			// Compensation 2: Cancel Order (if refund fails)
			structuredLog(EventSagaCompensationStarted, map[string]interface{}{
				"compensation_for": ShipOrderStep,
				"compensation":     CancelOrderCompensation,
				"order_id":         actualOrderID,
				"reason":           "Refund failed",
			})
			// As above, nil or specific body for cancel
			_, compErr2 := callService(ctx, fmt.Sprintf(orderServiceURL+cancelPathSuffix, actualOrderID), nil)
			if compErr2 != nil {
				structuredLog(EventSagaCompensationFailed, map[string]interface{}{
					"compensation":   CancelOrderCompensation,
					"order_id":       actualOrderID,
					"error":          compErr2.Error(),
					"original_error": fmt.Errorf("shipment failed (%v), refund failed (%v)", err, compErr1).Error(),
				})
				structuredLog(EventSagaFailed, map[string]interface{}{
					"order_id": actualOrderID,
					"reason":   "Shipment failed, refund failed, and order compensation failed",
				})
				return fmt.Sprintf("SAGA Failed: Shipment failed (%v), refund failed (%v), and order compensation also failed (%v)", err, compErr1, compErr2)
			}
			structuredLog(EventSagaFailed, map[string]interface{}{
				"order_id": actualOrderID,
				"reason":   "Shipment failed, refund failed, order compensated",
			})
			return fmt.Sprintf("SAGA Failed: Shipment failed (%v), refund failed (%v), order cancelled.", err, compErr1)
		}
		// Compensation 2: Cancel Order (after successful refund)
		structuredLog(EventSagaCompensationStarted, map[string]interface{}{
			"compensation_for": ShipOrderStep,
			"compensation":     CancelOrderCompensation,
			"order_id":         actualOrderID,
			"reason":           "Shipment failed, refund succeeded",
		})
		// As above, nil or specific body for cancel
		_, compErr2 := callService(ctx, fmt.Sprintf(orderServiceURL+cancelPathSuffix, actualOrderID), nil)
		if compErr2 != nil {
			structuredLog(EventSagaCompensationFailed, map[string]interface{}{
				"compensation":   CancelOrderCompensation,
				"order_id":       actualOrderID,
				"error":          compErr2.Error(),
				"original_error": fmt.Errorf("shipment failed (%v), refund succeeded", err).Error(),
			})
			structuredLog(EventSagaFailed, map[string]interface{}{
				"order_id": actualOrderID,
				"reason":   "Shipment failed, refund succeeded, and order compensation failed",
			})
			return fmt.Sprintf("SAGA Failed: Shipment failed (%v), refund succeeded, but order compensation also failed (%v)", err, compErr2)
		}
		structuredLog(EventSagaFailed, map[string]interface{}{
			"order_id": actualOrderID,
			"reason":   "Shipment failed, payment refunded, order compensated",
		})
		return fmt.Sprintf("SAGA Failed: Shipment failed (%v), payment refunded, order cancelled.", err)
	}

	structuredLog(EventSagaCompleted, map[string]interface{}{
		"order_id": actualOrderID, // Now logging the actual generated ID
		"status":   "completed",
	})
	return "SAGA Completed Successfully!"
}

// Handler for orchestrate saga request
func orchestrateSagaHandler(w http.ResponseWriter, r *http.Request) {
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
	defer func() {
		if closeErr := r.Body.Close(); closeErr != nil {
			log.Printf("Warning: Error closing request body: %v", closeErr)
		}
	}()

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second) // Saga timeout
	defer cancel()

	// Pass the OrderID received from the frontend (it might be empty, that's fine as order-service generates it)
	result := executeSaga(ctx, req.OrderID, req.Amount)

	resp := OrderResponse{
		OrderID: req.OrderID, // This will be the initial ID from the frontend, not the one generated by order-service.
		Status:  "processed",
		Message: result,
	}

	w.Header().Set(ContentTypeHeader, ApplicationJSON)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Error encoding response for OrderID %s: %v", req.OrderID, err)
		// Since headers are already sent, we can only log the error.
	}

	structuredLog(EventResponseSent, map[string]interface{}{
		"status":   http.StatusOK,
		"order_id": req.OrderID, // This will still log the initial ID from the frontend
		"message":  result,
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
	http.HandleFunc("/saga", orchestrateSagaHandler)
	http.HandleFunc("/health", healthCheckHandler) // New health check endpoint

	structuredLog("server_start", map[string]interface{}{"port": orchestratorPort})
	if err := http.ListenAndServe(":"+orchestratorPort, nil); err != nil {
		structuredLog("server_error", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
}
