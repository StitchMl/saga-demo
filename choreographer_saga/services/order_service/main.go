package main

import (
	"encoding/json"
	"fmt"
	"github.com/StitchMl/saga-demo/common/events"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/StitchMl/saga-demo/choreographer_saga/shared"
)

// In-memory database for orders
var ordersDB = struct {
	sync.RWMutex
	Data map[string]events.Order
}{Data: make(map[string]events.Order)}

var eventBus *shared.EventBus
var inventoryServiceURL string

func main() {
	rabbitMQURL := os.Getenv("RABBITMQ_URL") // Default URL of RabbitMQ
	if rabbitMQURL == "" {
		// Provide a safe fallback or fatal error if not set
		log.Fatal("RABBITMQ_URL environment variable not set.")
	}

	inventoryServiceURL = os.Getenv("INVENTORY_SERVICE_URL")
	if inventoryServiceURL == "" {
		log.Fatal("INVENTORY_SERVICE_URL environment variable not set.")
	}

	var err error
	eventBus, err = shared.NewEventBus(rabbitMQURL)
	if err != nil {
		log.Fatalf("Order Service: Failed to create event bus: %v", err)
	}
	defer eventBus.Close() // Make sure to close the connection when the service stops

	// Subscribe to events relevant to the order service
	if err := eventBus.Subscribe(events.OrderApprovedEvent, handleOrderApprovedEvent); err != nil {
		log.Fatalf("Order Service: Failed to subscribe to OrderApprovedEvent: %v", err)
	}
	if err := eventBus.Subscribe(events.OrderRejectedEvent, handleOrderRejectedEvent); err != nil {
		log.Fatalf("Order Service: Failed to subscribe to OrderRejectedEvent: %v", err)
	}

	http.HandleFunc("/create_order", createOrderHandler)

	log.Println("Order Service started on port 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// createOrderHandler starts the SAGA by issuing the OrderCreatedEvent
func createOrderHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var order events.Order
	err := json.NewDecoder(r.Body).Decode(&order)
	if err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Retrieve product prices from the Inventory service
	productIDs := make([]string, len(order.Items))
	for i, item := range order.Items {
		productIDs[i] = item.ProductID
	}

	prices, err := getPricesFromInventoryService(productIDs)
	if err != nil {
		log.Printf("Order Service: Failed to get product prices from Inventory Service: %v", err)
		http.Error(w, "Failed to retrieve product prices: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Enrich order items with prices
	for i, item := range order.Items {
		if price, ok := prices[item.ProductID]; ok {
			order.Items[i].Price = price
		} else {
			log.Printf("Order Service: Price not found for product %s", item.ProductID)
			http.Error(w, fmt.Sprintf("Price not found for product %s", item.ProductID), http.StatusBadRequest)
			return
		}
	}

	order.OrderID = fmt.Sprintf("order-%d", time.Now().UnixNano())
	order.Status = "pending" // Initial status

	ordersDB.Lock()
	ordersDB.Data[order.OrderID] = order
	ordersDB.Unlock()

	log.Printf("Order Service: Request received: Order creation %s for Customer %s with %d items", order.OrderID, order.CustomerID, len(order.Items))

	// Post the OrderCreated event to start the choreography saga
	payload := events.OrderCreatedPayload{
		OrderID:    order.OrderID,
		Items:      order.Items,
		CustomerID: order.CustomerID,
	}
	if err := eventBus.Publish(events.NewGenericEvent(events.OrderCreatedEvent, order.OrderID, "New order created", payload)); err != nil {
		log.Printf("Order Service: Error publishing OrderCreatedEvent: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}

	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(map[string]string{"message": "Order received, SAGA initiated via event", "order_id": order.OrderID}); err != nil {
		log.Printf("Order Service: Error encoding response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// getPricesFromInventoryService makes an HTTP call to the Inventory Service to get prices.
func getPricesFromInventoryService(productIDs []string) (map[string]float64, error) {
	if len(productIDs) == 0 {
		return nil, nil // No ID, no price to recover
	}

	// Construct the query string with all product_ids
	queryParams := make([]string, len(productIDs))
	for i, id := range productIDs {
		queryParams[i] = fmt.Sprintf("product_id=%s", id)
	}
	url := fmt.Sprintf("%s/products/prices?%s", inventoryServiceURL, strings.Join(queryParams, "&"))

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to call Inventory Service: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("Order Service: Error closing response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("inventory Service returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var prices map[string]float64
	if err := json.NewDecoder(resp.Body).Decode(&prices); err != nil {
		return nil, fmt.Errorf("failed to decode prices from Inventory Service response: %w", err)
	}
	return prices, nil
}

// handleOrderApprovedEvent handles the approval of the order
func handleOrderApprovedEvent(eventPayload interface{}) {
	payloadBytes, err := json.Marshal(eventPayload)
	if err != nil {
		log.Printf("Order Service: Error marshalling eventPayload for OrderApprovedEvent: %v", err)
		return
	}

	var payload events.OrderApprovedPayload
	err = json.Unmarshal(payloadBytes, &payload)
	if err != nil {
		log.Printf("Order Service: Error unmarshalling payload bytes to OrderApprovedPayload: %v", err)
		return
	}

	ordersDB.Lock()
	defer ordersDB.Unlock()

	order, exists := ordersDB.Data[payload.OrderID]
	if exists {
		order.Status = "approved"
		ordersDB.Data[payload.OrderID] = order
		log.Printf("Order Service: Order %s status updated to 'approved'.", payload.OrderID)
	} else {
		log.Printf("Order Service: Order %s not found for approval.", payload.OrderID)
	}
}

// handleOrderRejectedEvent handles order rejection (and potential compensation if not already handled by services)
func handleOrderRejectedEvent(eventPayload interface{}) {
	payloadBytes, err := json.Marshal(eventPayload)
	if err != nil {
		log.Printf("Order Service: Error marshalling eventPayload for OrderRejectedEvent: %v", err)
		return
	}

	var payload events.OrderRejectedPayload
	err = json.Unmarshal(payloadBytes, &payload)
	if err != nil {
		log.Printf("Order Service: Error unmarshalling payload bytes to OrderRejectedPayload: %v", err)
		return
	}

	ordersDB.Lock()
	defer ordersDB.Unlock()

	order, exists := ordersDB.Data[payload.OrderID]
	if exists {
		order.Status = "rejected"
		ordersDB.Data[payload.OrderID] = order
		log.Printf("Order Service: Order %s status updated to 'rejected' due to: %s", payload.OrderID, payload.Reason)
	} else {
		log.Printf("Order Service: Order %s not found for rejection.", payload.OrderID)
	}
}
