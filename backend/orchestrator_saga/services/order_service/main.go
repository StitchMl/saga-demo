package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	inventorydb "github.com/StitchMl/saga-demo/common/data_store"
	events "github.com/StitchMl/saga-demo/common/types"
)

const (
	contentTypeJSON = "application/json"
	contentType     = "Content-Type"
)

func main() {
	inventorydb.InitDB()
	http.HandleFunc("/create_order", createOrderHandler)
	http.HandleFunc("/orders/", getOrderHandler)
	http.HandleFunc("/orders", listOrdersHandler)
	http.HandleFunc("/products/prices", getPricesHandler)
	http.HandleFunc("/confirm", confirmOrderHandler)
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

// getOrderHandler retrieves an order by its ID.
func getOrderHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/orders/")
	if order, ok := inventorydb.GetOrder(id); ok {
		_ = json.NewEncoder(w).Encode(order)
		return
	}
	http.Error(w, "order not found", http.StatusNotFound)
}

// getPricesHandler retrieves the prices of products based on product IDs provided in the query parameters.
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
	w.Header().Set(contentType, contentTypeJSON)
	_ = json.NewEncoder(w).Encode(result)
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

	w.Header().Set(contentType, contentTypeJSON)
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
	if !exists {
		inventorydb.DB.Orders.Data[req.OrderID] = events.Order{
			OrderID: req.OrderID, Status: req.Status}
	} else {
		order.Status = req.Status
		inventorydb.DB.Orders.Data[req.OrderID] = order
	}
	inventorydb.DB.Orders.Unlock()

	w.Header().Set(contentType, contentTypeJSON)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Order status updated",
	})
}
