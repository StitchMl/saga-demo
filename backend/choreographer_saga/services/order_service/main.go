package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/StitchMl/saga-demo/choreographer_saga/shared"
	inventorydb "github.com/StitchMl/saga-demo/common/data_store"
	events "github.com/StitchMl/saga-demo/common/types"
)

var (
	eventBus            *shared.EventBus
	inventoryServiceURL string
)

func main() {
	// Initialise the global data store
	inventorydb.InitDB()

	rabbitMQURL := os.Getenv("RABBITMQ_URL")
	inventoryServiceURL = os.Getenv("INVENTORY_SERVICE_URL")
	port := os.Getenv("ORDER_SERVICE_PORT")
	if rabbitMQURL == "" || inventoryServiceURL == "" || port == "" {
		log.Fatal("Missing env: RABBITMQ_URL, INVENTORY_SERVICE_URL, ORDER_SERVICE_PORT")
	}

	var err error
	eventBus, err = shared.NewEventBus(rabbitMQURL)
	if err != nil {
		log.Fatalf("Unable to create EventBus: %v", err)
	}
	defer eventBus.Close()

	// Subscriptions
	subscribe(events.OrderApprovedEvent, handleOrderApprovedEvent)
	subscribe(events.OrderRejectedEvent, handleOrderRejectedEvent)

	// REST endpoints
	http.HandleFunc("/create_order", createOrderHandler)
	http.HandleFunc("/orders/", getOrderHandler)
	http.HandleFunc("/orders", listOrdersHandler)
	http.HandleFunc("/products/prices", getPricesHandler)
	http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Choreographer Order Service OK"))
	})

	log.Printf("Choreographer Order Service listening on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// subscribe: utility to subscribe to events with error handling
func subscribe(t events.EventType, h func(interface{})) {
	if err := eventBus.Subscribe(t, h); err != nil {
		log.Fatalf("Subscription error %s: %v", t, err)
	}
}

// listOrdersHandler: returns all orders
func listOrdersHandler(w http.ResponseWriter, r *http.Request) {
	cid := r.URL.Query().Get("customer_id")

	inventorydb.DB.Orders.RLock()
	out := make([]events.Order, 0, len(inventorydb.DB.Orders.Data))
	for _, o := range inventorydb.DB.Orders.Data {
		if cid == "" || o.CustomerID == cid {
			out = append(out, o)
		}
	}
	inventorydb.DB.Orders.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// getOrderHandler: retrieves a single order
func getOrderHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/orders/")
	if order, ok := inventorydb.GetOrder(id); ok {
		_ = json.NewEncoder(w).Encode(order)
		return
	}
	http.Error(w, "order not found", http.StatusNotFound)
}

// createOrderHandler: create the PENDING order and publish the Saga start event.
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

	// Retrieve prices from the Inventory service
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

	// *** WRITING in the shared data store ***
	inventorydb.DB.Orders.Lock()
	inventorydb.DB.Orders.Data[order.OrderID] = order
	inventorydb.DB.Orders.Unlock()

	payload := events.OrderCreatedPayload{
		OrderID:    order.OrderID,
		Items:      order.Items,
		CustomerID: order.CustomerID,
	}

	if err := eventBus.Publish(
		events.NewGenericEvent(events.OrderCreatedEvent, order.OrderID, "New order created", payload),
	); err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"message":  "Order received, SAGA initiated",
		"order_id": order.OrderID,
	})
}

// getPricesHandler for consistency with an orchestrator version
func getPricesHandler(w http.ResponseWriter, r *http.Request) {
	ids := r.URL.Query()["product_id"]
	if len(ids) == 0 {
		http.Error(w, "product_id param required", http.StatusBadRequest)
		return
	}
	result := make(map[string]float64, len(ids))
	for _, id := range ids {
		if p, ok := inventorydb.GetProductPrice(id); ok {
			result[id] = p
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// handleOrderApprovedEvent: update status -> approved
func handleOrderApprovedEvent(eventPayload interface{}) {
	var payload events.OrderStatusUpdatePayload
	if err := mapToStruct(eventPayload, &payload); err != nil {
		log.Printf("Order Service: Payload error OrderApprovedEvent: %v", err)
		return
	}

	inventorydb.DB.Orders.Lock()
	if order, exists := inventorydb.DB.Orders.Data[payload.OrderID]; exists {
		order.Status = "approved"
		inventorydb.DB.Orders.Data[payload.OrderID] = order
		log.Printf("Order Service: Order %s approved.", payload.OrderID)
	} else {
		log.Printf("Order Service: Order %s not found for approval.", payload.OrderID)
	}
	inventorydb.DB.Orders.Unlock()
}

// handleOrderRejectedEvent: update status -> rejected
func handleOrderRejectedEvent(eventPayload interface{}) {
	var payload events.OrderStatusUpdatePayload
	if err := mapToStruct(eventPayload, &payload); err != nil {
		log.Printf("Order Service: Payload error OrderRejectedEvent: %v", err)
		return
	}

	inventorydb.DB.Orders.Lock()
	if order, exists := inventorydb.DB.Orders.Data[payload.OrderID]; exists {
		order.Status = "rejected"
		inventorydb.DB.Orders.Data[payload.OrderID] = order
		log.Printf("Order Service: Order %s rejected (%s).", payload.OrderID, payload.Reason)
	} else {
		// If not found, create it with a rejected status per a track.
		inventorydb.DB.Orders.Data[payload.OrderID] = events.Order{
			OrderID: payload.OrderID,
			Status:  "rejected",
		}
		log.Printf("Order Service: Created placeholder for missing rejected order %s.", payload.OrderID)
	}
	inventorydb.DB.Orders.Unlock()
}

// mapToStruct: utility to convert a generic payload into a specific struct.
func mapToStruct(src, dst interface{}) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

// getPricesFromInventoryService: retrieve product prices from the Inventory service
func getPricesFromInventoryService(productIDs []string) (map[string]float64, error) {
	if inventoryServiceURL == "" {
		return nil, fmt.Errorf("INVENTORY_SERVICE_URL not set")
	}
	if len(productIDs) == 0 {
		return map[string]float64{}, nil
	}
	params := strings.Join(productIDs, "&product_id=")
	url := fmt.Sprintf("%s/products/prices?product_id=%s", inventoryServiceURL, params)

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("inventory service error: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("inventory status %d: %s", resp.StatusCode, body)
	}

	var prices map[string]float64
	if err := json.NewDecoder(resp.Body).Decode(&prices); err != nil {
		return nil, fmt.Errorf("decode error: %w", err)
	}
	return prices, nil
}
