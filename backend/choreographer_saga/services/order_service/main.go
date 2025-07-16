package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/StitchMl/saga-demo/choreographer_saga/shared"
	"github.com/StitchMl/saga-demo/common/types"
)

// In-memory database for orders
var ordersDB = struct {
	sync.RWMutex
	Data map[string]events.Order
}{Data: make(map[string]events.Order)}

var eventBus *shared.EventBus
var inventoryServiceURL string

func main() {
	rabbitMQURL := os.Getenv("RABBITMQ_URL")
	inventoryServiceURL = os.Getenv("INVENTORY_SERVICE_URL")
	port := os.Getenv("ORDER_SERVICE_PORT")
	if rabbitMQURL == "" || inventoryServiceURL == "" || port == "" {
		log.Fatal("Missing environment variables: RABBITMQ_URL, INVENTORY_SERVICE_URL, ORDER_SERVICE_PORT")
	}

	var err error
	eventBus, err = shared.NewEventBus(rabbitMQURL)
	if err != nil {
		log.Fatalf("Unable to create EventBus: %v", err)
	}
	defer eventBus.Close()

	if err := eventBus.Subscribe(events.OrderApprovedEvent, handleOrderApprovedEvent); err != nil {
		log.Fatal("Unable to subscribe to OrderApprovedEvent:", err)
	}
	if err := eventBus.Subscribe(events.OrderRejectedEvent, handleOrderRejectedEvent); err != nil {
		log.Fatal("Unable to subscribe to OrderRejectedEvent:", err)
	}

	http.HandleFunc("/create_order", createOrderHandler)
	log.Printf("Order Service started on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// createOrderHandler starts the SAGA by issuing the OrderCreatedEvent
func createOrderHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var order events.Order
	if err := json.NewDecoder(r.Body).Decode(&order); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	productIDs := make([]string, len(order.Items))
	for i, item := range order.Items {
		productIDs[i] = item.ProductID
	}

	prices, err := getPricesFromInventoryService(productIDs)
	if err != nil {
		http.Error(w, "Price recovery error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	for i, item := range order.Items {
		price, ok := prices[item.ProductID]
		if !ok {
			http.Error(w, "Missing price for "+item.ProductID, http.StatusBadRequest)
			return
		}
		order.Items[i].Price = price
	}

	order.OrderID = fmt.Sprintf("order-%d", time.Now().UnixNano())
	order.Status = "pending"

	ordersDB.Lock()
	ordersDB.Data[order.OrderID] = order
	ordersDB.Unlock()

	payload := events.OrderCreatedPayload{
		OrderID:    order.OrderID,
		Items:      order.Items,
		CustomerID: order.CustomerID,
	}
	if err := eventBus.Publish(events.NewGenericEvent(events.OrderCreatedEvent, order.OrderID, "New order created", payload)); err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"messaggio": "Order received, SAGA initiated",
		"order_id":  order.OrderID,
	})
}

// getPricesFromInventoryService makes an HTTP call to the Inventory Service to get prices.
func getPricesFromInventoryService(productIDs []string) (map[string]float64, error) {
	if len(productIDs) == 0 {
		return nil, nil
	}
	params := strings.Join(productIDs, "&product_id=")
	url := fmt.Sprintf("%s/products/prices?product_id=%s", inventoryServiceURL, params)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("inventory Service error: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("inventory Service status %d: %s", resp.StatusCode, body)
	}

	var prices map[string]float64
	if err := json.NewDecoder(resp.Body).Decode(&prices); err != nil {
		return nil, fmt.Errorf("decode error: %w", err)
	}
	return prices, nil
}

// handleOrderApprovedEvent handles the approval of the order
func handleOrderApprovedEvent(eventPayload interface{}) {
	var payload events.OrderStatusUpdatePayload
	if err := mapToStruct(eventPayload, &payload); err != nil {
		log.Printf("Order Service: Payload error OrderApprovedEvent: %v", err)
		return
	}

	ordersDB.Lock()
	defer ordersDB.Unlock()

	if order, exists := ordersDB.Data[payload.OrderID]; exists {
		order.Status = "approved"
		ordersDB.Data[payload.OrderID] = order
		log.Printf("Order Service: Order %s approved.", payload.OrderID)
	} else {
		log.Printf("Order Service: Order %s not found.", payload.OrderID)
	}
}

// mapToStruct converts an interface to a struct via JSON
func mapToStruct(src, dst interface{}) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

// handleOrderRejectedEvent handles order rejection (and potential compensation if not already handled by services)
func handleOrderRejectedEvent(eventPayload interface{}) {
	var payload events.OrderStatusUpdatePayload
	if err := mapToStruct(eventPayload, &payload); err != nil {
		log.Printf("Order Service: Payload error OrderRejectedEvent: %v", err)
		return
	}

	ordersDB.Lock()
	defer ordersDB.Unlock()

	if order, exists := ordersDB.Data[payload.OrderID]; exists {
		order.Status = "rejected"
		ordersDB.Data[payload.OrderID] = order
		log.Printf("Order Service: Order %s status updated to 'rejected' due to: %s", payload.OrderID, payload.Reason)
	} else {
		log.Printf("Order Service: Order %s not found for rejection.", payload.OrderID)
	}
}
