package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	events "github.com/StitchMl/saga-demo/common/types"
)

const (
	contentTypeJSON = "application/json"
	contentType     = "Content-Type"
)

// OrdersDB is an in-memory database for the order service.
var OrdersDB = struct {
	sync.RWMutex
	Data map[string]events.Order
}{Data: make(map[string]events.Order)}

func main() {
	http.HandleFunc("/create_order", createOrderHandler)
	http.HandleFunc("/orders/", getOrderHandler)
	http.HandleFunc("/orders", listOrdersHandler)
	http.HandleFunc("/update_status", updateOrderStatusHandler)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, "Orchestrator Order Service is healthy!")
	})

	port := os.Getenv("ORDER_SERVICE_PORT")
	if port == "" {
		log.Fatal("ORDER_SERVICE_PORT is not set")
	}

	log.Printf("Order Service listening on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// listOrdersHandler returns all orders
func listOrdersHandler(w http.ResponseWriter, r *http.Request) {
	cid := r.URL.Query().Get("customer_id")

	OrdersDB.RLock()
	out := make([]events.Order, 0, len(OrdersDB.Data))
	for _, o := range OrdersDB.Data {
		if cid == "" || o.CustomerID == cid {
			out = append(out, o)
		}
	}
	OrdersDB.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// getOrderHandler retrieves an order by its ID.
func getOrderHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/orders/")
	OrdersDB.RLock()
	defer OrdersDB.RUnlock()
	if order, ok := OrdersDB.Data[id]; ok {
		w.Header().Set(contentType, contentTypeJSON)
		_ = json.NewEncoder(w).Encode(order)
		return
	}
	http.Error(w, "order not found", http.StatusNotFound)
}

// createOrderHandler handles the initial order creation request from the Orchestrator.
func createOrderHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var order events.Order
	if err := json.NewDecoder(r.Body).Decode(&order); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		log.Printf("Order Service: Invalid request body: %v", err)
		return
	}

	if order.OrderID == "" {
		order.OrderID = fmt.Sprintf("order-%d", time.Now().UnixNano())
	}
	order.Status = "pending"

	OrdersDB.Lock()
	OrdersDB.Data[order.OrderID] = order
	OrdersDB.Unlock()

	log.Printf("Order Service: Created order %s for Customer %s. Status: %s", order.OrderID, order.CustomerID, order.Status)
	for _, item := range order.Items {
		log.Printf(" - Item: ProductID: %s, Quantity: %d", item.ProductID, item.Quantity)
	}

	w.Header().Set(contentType, contentTypeJSON)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"order_id": order.OrderID,
		"status":   "success",
		"message":  "Order created successfully",
	})
}

// updateOrderStatusHandler handles updating the status of an order.
func updateOrderStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req events.OrderStatusUpdatePayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	OrdersDB.Lock()
	defer OrdersDB.Unlock()
	order, exists := OrdersDB.Data[req.OrderID]
	if !exists {
		http.Error(w, "Order not found", http.StatusNotFound)
		return
	}

	log.Printf("Updating status for order %s from %s to %s. Reason: %s", req.OrderID, order.Status, req.Status, req.Reason)
	order.Status = req.Status
	order.Reason = req.Reason
	if req.Total > 0 {
		order.Total = req.Total
	}
	OrdersDB.Data[req.OrderID] = order

	w.Header().Set(contentType, contentTypeJSON)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Order status updated",
	})
}
