package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	events "github.com/StitchMl/saga-demo/common/types"
)

const (
	contentTypeJSON      = "application/json"
	contentType          = "Content-Type"
	errorInvalidCustomer = "Invalid customer"
)

// ServiceError defines a custom error for service call failures.
type ServiceError struct {
	URL     string
	Status  int
	Message string
}

func (e *ServiceError) Error() string {
	return fmt.Sprintf("service %s responded with status %d: %s", e.URL, e.Status, e.Message)
}

// Config Configuration of Services
type Config struct {
	OrderServiceURL     string `json:"order_service_url"`
	InventoryServiceURL string `json:"inventory_service_url"`
	PaymentServiceURL   string `json:"payment_service_url"`
	AuthServiceURL      string `json:"auth_service_url"`
	ServerPort          string `json:"server_port"`
	ServiceCallTimeout  time.Duration
}

var appConfig Config

type SagaEvent struct {
	OrderID   string    `json:"order_id"`
	Step      string    `json:"step"`
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Details   string    `json:"details,omitempty"`
}

// In-memory logging of SAGA events to track transaction status
var sagaLog = struct {
	sync.RWMutex
	Events map[string][]SagaEvent // Map OrderID to a list of events
}{Events: make(map[string][]SagaEvent)}

func main() {
	// Load configuration
	loadConfigFromEnv()

	// Endpoint to start a new order SAGA
	http.HandleFunc("/create_order", createOrderHandler)

	log.Printf("Orchestrator started on port %s", appConfig.ServerPort)
	log.Fatal(http.ListenAndServe(":"+appConfig.ServerPort, nil))
}

// Load configuration from a JSON file
func loadConfigFromEnv() {
	appConfig.OrderServiceURL = os.Getenv("OrderServiceURL")
	if appConfig.OrderServiceURL == "" {
		log.Fatal("Environment variable OrderServiceURL not set.")
	}
	appConfig.InventoryServiceURL = os.Getenv("InventoryServiceURL")
	if appConfig.InventoryServiceURL == "" {
		log.Fatal("Environment variable InventoryServiceURL not set.")
	}
	appConfig.PaymentServiceURL = os.Getenv("PaymentServiceURL")
	if appConfig.PaymentServiceURL == "" {
		log.Fatal("Environment variable PaymentServiceURL not set.")
	}
	appConfig.AuthServiceURL = os.Getenv("AuthServiceURL")
	if appConfig.AuthServiceURL == "" {
		log.Fatal("Environment variable AuthServiceURL not set.")
	}
	appConfig.ServerPort = os.Getenv("ServerPort")
	if appConfig.ServerPort == "" {
		log.Fatal("ServerPort environment variable not set! Must be set to run the server.")
	}
	timeoutStr := os.Getenv("SERVICE_CALL_TIMEOUT_SECONDS")
	if timeoutStr == "" {
		timeoutStr = "10" // Default timeout
	}
	timeout, err := time.ParseDuration(timeoutStr + "s")
	if err != nil {
		log.Fatalf("Invalid SERVICE_CALL_TIMEOUT_SECONDS: %v", err)
	}
	appConfig.ServiceCallTimeout = timeout

	log.Printf("Configuration loaded: %+v", appConfig)
}

// Order creation manager (starts SAGA)
func createOrderHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Use events.Order for the incoming request
	var order events.Order
	err := json.NewDecoder(r.Body).Decode(&order)
	if err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Assigns an ID to the order and sets the initial status.
	order.OrderID = fmt.Sprintf("order-%d", time.Now().UnixNano())
	order.Status = "pending"

	// Initial log, adapted for the new items format
	log.Printf("Request received: Order creation %s for Customer %s, Items: %+v", order.OrderID, order.CustomerID, order.Items)

	// It starts the SAGA synchronously to provide immediate feedback.
	finalOrder, err := startSaga(order)
	if err != nil {
		// SAGA failed, respond with an error status, and the final order states.
		w.Header().Set(contentType, contentTypeJSON)
		w.WriteHeader(http.StatusConflict) // 409 Conflict is a good code for a business rule failure.
		if err := json.NewEncoder(w).Encode(finalOrder); err != nil {
			log.Printf("Error in the encoding of the JSON response: %v", err)
		}
		return
	}

	// SAGA succeeded
	w.Header().Set(contentType, contentTypeJSON)
	w.WriteHeader(http.StatusOK) // 200 OK for success
	if err := json.NewEncoder(w).Encode(finalOrder); err != nil {
		log.Printf("Error in the encoding of the JSON response: %v", err)
	}
}

// Start the SAGA logic
func startSaga(order events.Order) (events.Order, error) {
	logSagaEvent(order.OrderID, "SAGA_START", "started", "Saga started for order.")

	// Step 1: Create Order in Order Service with “pending” status
	logSagaEvent(order.OrderID, "CREATE_ORDER", "started", "Creating order in order service.")
	resp, err := makeServiceCall(appConfig.OrderServiceURL+"/create_order", order)
	if err != nil || resp["status"] != "success" {
		log.Printf("Failed to create order %s in order service: %v, response: %+v", order.OrderID, err, resp)
		logSagaEvent(order.OrderID, "CREATE_ORDER", "failed", "Failed to create order.")
		order.Status = "failed"
		order.Reason = "Failed to create order record"
		return order, fmt.Errorf("failed to create order")
	}
	logSagaEvent(order.OrderID, "CREATE_ORDER", "completed", "Order created successfully in order service.")

	// Step 2: Validate Customer
	logSagaEvent(order.OrderID, "VALIDATE_CUSTOMER", "started", "Validating customer.")
	authReq := map[string]interface{}{"customer_id": order.CustomerID}
	authResp, err := makeServiceCall(appConfig.AuthServiceURL+"/validate", authReq)
	if err != nil {
		log.Printf("Customer validation failed for order %s: %v", order.OrderID, err)
		logSagaEvent(order.OrderID, "VALIDATE_CUSTOMER", "failed", "Customer validation failed.")
		updateOrderStatus(order.OrderID, "rejected", errorInvalidCustomer, &order.Total)
		order.Status = "rejected"
		order.Reason = getCleanErrorMessage(err, "Customer validation failed")
		return order, err
	}
	if valid, ok := authResp["valid"].(bool); !ok || !valid {
		log.Printf("Customer validation returned not valid for order %s: response: %+v", order.OrderID, authResp)
		logSagaEvent(order.OrderID, "VALIDATE_CUSTOMER", "failed", "Customer validation returned false.")
		updateOrderStatus(order.OrderID, "rejected", errorInvalidCustomer, &order.Total)
		order.Status = "rejected"
		order.Reason = errorInvalidCustomer
		return order, fmt.Errorf("customer validation returned false")
	}
	logSagaEvent(order.OrderID, "VALIDATE_CUSTOMER", "completed", "Customer validated successfully.")

	// Step 3: Get product prices and calculate the total amount
	logSagaEvent(order.OrderID, "GET_PRICES", "started", "Getting product prices from inventory service.")
	totalAmount, err := getPricesAndCalculateTotal(order.Items)
	if err != nil {
		log.Printf("Failed to get prices for order %s: %v", order.OrderID, err)
		logSagaEvent(order.OrderID, "GET_PRICES", "failed", fmt.Sprintf("Failed to get prices: %v", err))
		compensateSaga(order.OrderID, order, "get_prices_failure")
		order.Status = "rejected"
		order.Reason = getCleanErrorMessage(err, "Failed to get prices")
		return order, err
	}
	order.Total = totalAmount
	log.Printf("Calculated total amount for Order %s: %.2f", order.OrderID, totalAmount)
	logSagaEvent(order.OrderID, "GET_PRICES", "completed", "Prices obtained and total calculated.")

	// Step 4: Reserve Products in the Inventory
	logSagaEvent(order.OrderID, "RESERVE_INVENTORY", "started", "Attempting to reserve inventory.")
	// Pass the entire list of items for the reserve
	reserveReq := events.InventoryRequestPayload{
		OrderID: order.OrderID,
		Items:   order.Items,
	}
	resp, err = makeServiceCall(appConfig.InventoryServiceURL+"/reserve", reserveReq)
	if err != nil || resp["status"] != "success" {
		log.Printf("Inventory reserve failure for order %s: %v, response: %+v", order.OrderID, err, resp)
		logSagaEvent(order.OrderID, "RESERVE_INVENTORY", "failed", fmt.Sprintf("Inventory reservation failed: %v", err))
		compensateSaga(order.OrderID, order, "inventory_failure")
		order.Status = "rejected"
		order.Reason = getCleanErrorMessage(err, "Inventory reservation failed")
		return order, err
	}
	log.Printf("Successfully reserved inventory for order %s", order.OrderID)
	logSagaEvent(order.OrderID, "RESERVE_INVENTORY", "completed", "Inventory reserved successfully.")

	// Step 5: Process Payment
	logSagaEvent(order.OrderID, "PROCESS_PAYMENT", "started", "Attempting to process payment.")
	paymentReq := events.PaymentPayload{
		OrderID:    order.OrderID,
		CustomerID: order.CustomerID,
		Amount:     totalAmount,
	}
	resp, err = makeServiceCall(appConfig.PaymentServiceURL+"/process", paymentReq)
	if err != nil || resp["status"] != "success" {
		log.Printf("Failure to process payment for order %s: %v, response: %+v", order.OrderID, err, resp)
		logSagaEvent(order.OrderID, "PROCESS_PAYMENT", "failed", fmt.Sprintf("Payment processing failed: %v", err))
		compensateSaga(order.OrderID, order, "payment_failure")
		order.Status = "rejected"
		order.Reason = getCleanErrorMessage(err, "Payment processing failed")
		return order, err
	}
	log.Printf("Payment successfully processed for order %s", order.OrderID)
	logSagaEvent(order.OrderID, "PROCESS_PAYMENT", "completed", "Payment processed successfully.")

	// Step 6: Order Confirmation
	logSagaEvent(order.OrderID, "CONFIRM_ORDER", "started", "Attempting to confirm order.")
	if !updateOrderStatus(order.OrderID, "approved", "Saga completed successfully", &order.Total) {
		log.Printf("Order confirmation failure for order %s", order.OrderID)
		logSagaEvent(order.OrderID, "CONFIRM_ORDER", "failed", "Order confirmation failed, requires manual intervention.")
		order.Status = "failed_confirmation"
		order.Reason = "Order confirmation failed, requires manual intervention."
		return order, fmt.Errorf("order confirmation failed")
	}
	log.Printf("Order %s successfully completed!", order.OrderID)
	logSagaEvent(order.OrderID, "SAGA_COMPLETE", "completed", "Order saga completed successfully.")
	order.Status = "approved"
	order.Reason = "Saga completed successfully"
	return order, nil
}

// The compensateSaga function now receives the full order object
func compensateSaga(orderID string, order events.Order, reason string) {
	log.Printf("Start of compensation for order %s due to: %s", orderID, reason)
	logSagaEvent(orderID, "SAGA_COMPENSATION", "started", fmt.Sprintf("Compensation initiated due to %s", reason))

	sagaLog.RLock()
	eventsLogged := sagaLog.Events[orderID]
	sagaLog.RUnlock()

	// Iterate events in reverse order to compensate
	for i := len(eventsLogged) - 1; i >= 0; i-- {
		event := eventsLogged[i]
		if event.Status == "completed" {
			switch event.Step {
			case "PROCESS_PAYMENT":
				revertPayment(orderID, reason)
			case "RESERVE_INVENTORY":
				// Pass the entire list of items to inventory clearing
				cancelInventoryReservation(orderID, order.Items, reason)
			case "CREATE_ORDER":
				updateOrderStatus(orderID, "rejected", reason, &order.Total)
			}
		}
	}
	log.Printf("SAGA compensation for order %s completed.", orderID)
	logSagaEvent(orderID, "SAGA_COMPENSATION", "completed", "Saga compensation completed.")
}

// getPricesAndCalculateTotal fetches prices from the inventory service and calculates the total.
func getPricesAndCalculateTotal(items []events.OrderItem) (float64, error) {
	var totalAmount float64
	for i, item := range items {
		priceReq := map[string]string{"product_id": item.ProductID}
		resp, err := makeServiceCall(appConfig.InventoryServiceURL+"/get_price", priceReq)
		if err != nil {
			return 0, fmt.Errorf("could not get price for product %s: %w", item.ProductID, err)
		}
		priceStr, ok := resp["price"].(string)
		if !ok {
			return 0, fmt.Errorf("price for product %s is not a string: %+v", item.ProductID, resp)
		}
		price, err := strconv.ParseFloat(priceStr, 64)
		if err != nil {
			return 0, fmt.Errorf("could not parse price for product %s from response: %+v", item.ProductID, resp)
		}
		items[i].Price = price
		if items[i].Price <= 0 {
			return 0, fmt.Errorf("product %s has an invalid price: %.2f", item.ProductID, items[i].Price)
		}
		totalAmount += items[i].Price * float64(item.Quantity)
	}
	return totalAmount, nil
}

// Helper function to update order status
func updateOrderStatus(orderID, status, reason string, total *float64) bool {
	logSagaEvent(orderID, "UPDATE_ORDER_STATUS", "started", fmt.Sprintf("Updating order status to %s", status))
	updateReq := events.OrderStatusUpdatePayload{
		OrderID: orderID,
		Status:  status,
		Reason:  reason,
	}
	if total != nil {
		updateReq.Total = *total
	}
	resp, err := makeServiceCall(appConfig.OrderServiceURL+"/update_status", updateReq)
	if err != nil || resp["status"] != "success" {
		log.Printf("Error updating order status for %s: %v, response: %+v", orderID, err, resp)
		logSagaEvent(orderID, "UPDATE_ORDER_STATUS", "failed", fmt.Sprintf("Failed to update order status: %v", err))
		return false
	}
	log.Printf("Order status for %s updated to %s.", orderID, status)
	logSagaEvent(orderID, "UPDATE_ORDER_STATUS", "completed", fmt.Sprintf("Order status updated to %s", status))
	return true
}

// Helper function to offset payment
func revertPayment(orderID string, reason string) {
	logSagaEvent(orderID, "REVERT_PAYMENT", "compensating", "Attempting to revert payment.")
	revertReq := map[string]interface{}{
		"order_id": orderID,
		"reason":   reason,
	}
	resp, err := makeServiceCall(appConfig.PaymentServiceURL+"/revert", revertReq)
	if err != nil || resp["status"] != "success" {
		log.Printf("Failure to offset payment for order %s: %v, response: %+v", orderID, err, resp)
		logSagaEvent(orderID, "REVERT_PAYMENT", "failed", "Payment reversion failed, manual intervention might be needed.")
	} else {
		log.Printf("Payment for order %s successfully compensated.", orderID)
		logSagaEvent(orderID, "REVERT_PAYMENT", "compensated", "Payment reverted successfully.")
	}
}

// Helper function to cancel inventory reservation
func cancelInventoryReservation(orderID string, items []events.OrderItem, reason string) {
	logSagaEvent(orderID, "CANCEL_RESERVATION", "compensating", "Attempting to cancel inventory reservation.")
	cancelReq := events.InventoryRequestPayload{
		OrderID: orderID,
		Items:   items,
		Reason:  reason,
	}
	resp, err := makeServiceCall(appConfig.InventoryServiceURL+"/cancel_reservation", cancelReq)
	if err != nil || resp["status"] != "success" {
		log.Printf("Inventory compensation failure for order %s: %v, response: %+v", orderID, err, resp)
		logSagaEvent(orderID, "CANCEL_RESERVATION", "failed", "Inventory reservation cancellation failed, manual intervention might be needed.")
	} else {
		log.Printf("Inventory reserve for order %s successfully compensated.", orderID)
		logSagaEvent(orderID, "CANCEL_RESERVATION", "compensated", "Inventory reservation cancelled successfully.")
	}
}

// Function for making HTTP calls to services
func makeServiceCall(url string, payload interface{}) (map[string]interface{}, error) {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("payload marshalling error: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("error when creating HTTP request: %w", err)
	}
	req.Header.Set(contentType, contentTypeJSON)

	client := &http.Client{Timeout: appConfig.ServiceCallTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error in request to service %s: %w", url, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Warning: Error closing HTTP response body from %s: %v", url, err)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error in reading the answer: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errorResult map[string]interface{}
		_ = json.Unmarshal(body, &errorResult)
		errorMessage, _ := errorResult["message"].(string)
		if errorMessage == "" {
			errorMessage = string(body)
		}
		return nil, &ServiceError{URL: url, Status: resp.StatusCode, Message: errorMessage}
	}

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		// Log the raw body if JSON unmarshalling fails to aid debugging
		log.Printf("Error unmarshalling JSON response from %s. Raw body: %s. Error: %v", url, string(body), err)
		return nil, fmt.Errorf("error in parsing the JSON response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return result, fmt.Errorf("the service %s answered with status %d: %s", url, resp.StatusCode, body)
	}

	return result, nil
}

// Log an event in the SAGA log
func logSagaEvent(orderID, step, status, details string) {
	event := SagaEvent{
		OrderID:   orderID,
		Step:      step,
		Status:    status,
		Timestamp: time.Now(),
		Details:   details,
	}

	sagaLog.Lock()
	sagaLog.Events[orderID] = append(sagaLog.Events[orderID], event)
	sagaLog.Unlock()

	log.Printf("[SAGA Event] Order: %s, Step: %s, Status: %s, Details: %s", orderID, step, status, details)
}

// getCleanErrorMessage extracts a user-friendly message from a ServiceError.
func getCleanErrorMessage(err error, defaultMessage string) string {
	var serviceErr *ServiceError
	if errors.As(err, &serviceErr) {
		return serviceErr.Message
	}
	if err != nil {
		return err.Error()
	}
	return defaultMessage
}
