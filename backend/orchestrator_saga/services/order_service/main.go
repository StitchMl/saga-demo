package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	inventorydb "github.com/StitchMl/saga-demo/common/data_store"
	"github.com/StitchMl/saga-demo/common/types"
)

func main() {
	http.HandleFunc("/create_order", createOrderHandler)
	http.HandleFunc("/confirm", confirmOrderHandler)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, "Orchestrator Order Service is healthy!")
	})

	port := os.Getenv("ORDER_SERVICE_PORT")
	if port == "" {
		port = "8081"
	}

	log.Printf("Order Service listening on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
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

	inventorydb.DB.Orders.Lock()
	inventorydb.DB.Orders.Data[order.OrderID] = order
	inventorydb.DB.Orders.Unlock()

	log.Printf("Order Service: Created order %s for Customer %s. Status: %s", order.OrderID, order.CustomerID, order.Status)
	for _, item := range order.Items {
		log.Printf(" - Item: ProductID: %s, Quantity: %d", item.ProductID, item.Quantity)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"order_id": order.OrderID,
		"status":   order.Status,
		"message":  "Order created successfully",
	})
}

// confirmOrderHandler handles confirmation or updating the status of an order.
func confirmOrderHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		OrderID string `json:"order_id"`
		Status  string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	inventorydb.DB.Orders.Lock()
	order, exists := inventorydb.DB.Orders.Data[req.OrderID]
	if exists {
		order.Status = req.Status
		inventorydb.DB.Orders.Data[req.OrderID] = order
	}
	inventorydb.DB.Orders.Unlock()

	if !exists {
		http.Error(w, "Order not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Order status updated",
	})
}
