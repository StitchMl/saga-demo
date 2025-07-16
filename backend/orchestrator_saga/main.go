package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/StitchMl/saga-demo/common/data_store"
	"github.com/StitchMl/saga-demo/common/types"
)

// Config Configuration of Services
type Config struct {
	OrderServiceURL     string `json:"order_service_url"`
	InventoryServiceURL string `json:"inventory_service_url"`
	PaymentServiceURL   string `json:"payment_service_url"`
	ServerPort          string `json:"server_port"`
}

var appConfig Config

type SagaEvent struct {
	OrderID   string    `json:"order_id"`
	Step      string    `json:"step"`
	Status    string    `json:"status"` // Started, completed, failed, compensating, compensated
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

	// Initializes the inventory and price database
	inventorydb.InitDB()

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
	appConfig.ServerPort = os.Getenv("ServerPort")
	if appConfig.ServerPort == "" {
		appConfig.ServerPort = "8080" // Default port if not set
		log.Printf("ServerPort environment variable not set, using default: %s", appConfig.ServerPort)
	}

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

	// It starts the SAGA in a goroutine in order not to block the HTTP request.
	go startSaga(order)

	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(map[string]string{"message": "Ordine ricevuto, SAGA avviata", "order_id": order.OrderID}); err != nil {
		log.Printf("Error in the encoding of the JSON response: %v", err)
	}
}

// Start the SAGA logic
func startSaga(order events.Order) {
	logSagaEvent(order.OrderID, "ORDER_CREATED", "started", "Order service received create order request.")

	// Calculates the total amount of the order based on actual prices.
	totalAmount := 0.0
	for i, item := range order.Items {
		price, ok := inventorydb.GetProductPrice(item.ProductID)
		if !ok {
			log.Printf("Product price not found for Product %s in Order %s. Aborting SAGA.", item.ProductID, order.OrderID)
			logSagaEvent(order.OrderID, "GET_PRICE", "failed", fmt.Sprintf("Product price not found for %s", item.ProductID))
			compensateSaga(order.OrderID, order.Items, "price_lookup_failure") // Passa tutti gli item per la compensazione
			return
		}
		// Assigns the price to the item in order for consistency
		order.Items[i].Price = price
		totalAmount += price * float64(item.Quantity)
	}
	log.Printf("Calculated total amount for Order %s: %.2f", order.OrderID, totalAmount)

	// Step 1: Reserve Products in the Inventory
	logSagaEvent(order.OrderID, "RESERVE_INVENTORY", "started", "Attempting to reserve inventory.")
	// Pass the entire list of items for the reserve
	reserveReq := map[string]interface{}{
		"order_id": order.OrderID,
		"items":    order.Items, // Pass all items with quantity and price
	}
	resp, err := makeServiceCall(appConfig.InventoryServiceURL+"/reserve", reserveReq)
	if err != nil || resp["status"] != "success" {
		log.Printf("Inventory reserve failure for order %s: %v, response: %+v", order.OrderID, err, resp)
		logSagaEvent(order.OrderID, "RESERVE_INVENTORY", "failed", fmt.Sprintf("Inventory reservation failed: %v", err))
		compensateSaga(order.OrderID, order.Items, "inventory_failure") // Passes all items for compensation
		return
	}
	log.Printf("Successfully reserved inventory for order %s", order.OrderID)
	logSagaEvent(order.OrderID, "RESERVE_INVENTORY", "completed", "Inventory reserved successfully.")

	// Step 2: Process Payment
	logSagaEvent(order.OrderID, "PROCESS_PAYMENT", "started", "Attempting to process payment.")
	paymentReq := map[string]interface{}{
		"order_id":    order.OrderID,
		"customer_id": order.CustomerID,
		"amount":      totalAmount,
	}
	resp, err = makeServiceCall(appConfig.PaymentServiceURL+"/process", paymentReq)
	if err != nil || resp["status"] != "success" {
		log.Printf("Failure to process payment for order %s: %v, response: %+v", order.OrderID, err, resp)
		logSagaEvent(order.OrderID, "PROCESS_PAYMENT", "failed", fmt.Sprintf("Payment processing failed: %v", err))
		compensateSaga(order.OrderID, order.Items, "payment_failure") // Passa tutti gli item per la compensazione
		return
	}
	log.Printf("Payment successfully processed for order %s", order.OrderID)
	logSagaEvent(order.OrderID, "PROCESS_PAYMENT", "completed", "Payment processed successfully.")

	// Step 3: Order Confirmation
	logSagaEvent(order.OrderID, "CONFIRM_ORDER", "started", "Attempting to confirm order.")
	// The confirmation request should use events.Order or a derivation thereof for consistency.
	confirmReq := map[string]interface{}{
		"order_id":    order.OrderID,
		"status":      "approved",
		"items":       order.Items,
		"customer_id": order.CustomerID,
	}
	resp, err = makeServiceCall(appConfig.OrderServiceURL+"/confirm", confirmReq)
	if err != nil || resp["status"] != "success" {
		log.Printf("Order confirmation failure for order %s: %v, response: %+v", order.OrderID, err, resp)
		logSagaEvent(order.OrderID, "CONFIRM_ORDER", "failed", fmt.Sprintf("Order confirmation failed: %v", err))
		return
	}
	log.Printf("Order %s successfully completed!", order.OrderID)
	logSagaEvent(order.OrderID, "SAGA_COMPLETE", "completed", "Order saga completed successfully.")
}

// The compensateSaga function now receives a slice of types.OrderItem
func compensateSaga(orderID string, items []events.OrderItem, reason string) {
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
				cancelInventoryReservation(orderID, items, reason)
			}
		}
	}
	// Updates the final status of the order to 'rejected' in the Order service.
	logSagaEvent(orderID, "UPDATE_ORDER_STATUS_REJECTED", "started", "Updating order status to rejected.")
	updateReq := map[string]interface{}{
		"order_id": orderID,
		"status":   "rejected",
	}
	_, err := makeServiceCall(appConfig.OrderServiceURL+"/confirm", updateReq)
	if err != nil {
		log.Printf("Error updating order status %s to rejected: %v", orderID, err)
		logSagaEvent(orderID, "UPDATE_ORDER_STATUS_REJECTED", "failed", fmt.Sprintf("Failed to update order status to rejected: %v", err))
	} else {
		log.Printf("Order status %s updated to rejected.", orderID)
		logSagaEvent(orderID, "UPDATE_ORDER_STATUS_REJECTED", "completed", "Order status updated to rejected.")
	}

	log.Printf("SAGA compensation for order %s completed.", orderID)
	logSagaEvent(orderID, "SAGA_COMPENSATION", "completed", "Saga compensation completed.")
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
	cancelReq := map[string]interface{}{
		"order_id": orderID,
		"items":    items,
		"reason":   reason,
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
func makeServiceCall(url string, payload map[string]interface{}) (map[string]string, error) {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("payload marshalling error: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("error when creating HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
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

	var result map[string]string
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
