package main

import (
	"encoding/json"
	"github.com/StitchMl/saga-demo/common/events"
	inventorydb "github.com/StitchMl/saga-demo/common/inventory_db"
	"log"
	"net/http"
)

// InventoryRequest struct for reservation and deletion now supports multiple items
type InventoryRequest struct {
	OrderID string             `json:"order_id"`
	Items   []events.OrderItem `json:"items"`
	Reason  string             `json:"reason,omitempty"`
}

func main() {
	inventorydb.InitDB()

	http.HandleFunc("/reserve", reserveInventoryHandler)
	http.HandleFunc("/cancel_reservation", cancelReservationHandler)

	log.Println("Inventory Service started on port 8082")
	log.Fatal(http.ListenAndServe(":8082", nil))
}

// Manager to reserve products
func reserveInventoryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req InventoryRequest // Use the new struct
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		log.Printf("Invalid request body for /reserve: %v", err) // More detailed log
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	inventorydb.DB.Lock()
	defer inventorydb.DB.Unlock()

	for _, item := range req.Items { // Iter on all items
		available, exists := inventorydb.DB.Data[item.ProductID]
		if !exists || available < item.Quantity {
			log.Printf("Inventory reserve failure for order %s: Product %s not available or insufficient quantity. Available: %d, Required: %d", req.OrderID, item.ProductID, available, item.Quantity)
			w.WriteHeader(http.StatusBadRequest)
			if err := json.NewEncoder(w).Encode(map[string]string{"status": "failure", "message": "Insufficient quantity or no product"}); err != nil {
				printEncodeError(err, w)
			}
			return // It fails if even one item is not available
		}
	}

	// If all items are available, then proceed with the reserve
	for _, item := range req.Items {
		inventorydb.DB.Data[item.ProductID] -= item.Quantity
		log.Printf("Reserved %d units of Product %s for Order %s. Remaining inventory: %d", item.Quantity, item.ProductID, req.OrderID, inventorydb.DB.Data[item.ProductID])
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Inventory reserved"}); err != nil {
		printEncodeError(err, w)
	}
}

// Manager to cancel a reserve of products (compensation)
func cancelReservationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req InventoryRequest // Use the new struct
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		log.Printf("Invalid request body for /cancel_reservation: %v", err) // More detailed log
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	inventorydb.DB.Lock()
	defer inventorydb.DB.Unlock()

	for _, item := range req.Items { // Itera on all items for compensation
		inventorydb.DB.Data[item.ProductID] += item.Quantity
		log.Printf("Cancelled reservation of %d units of Product %s for Order %s (reason: %s). Inventory restored: %d", item.Quantity, item.ProductID, req.OrderID, req.Reason, inventorydb.DB.Data[item.ProductID])
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Reserve cancelled and inventory restored"}); err != nil {
		printEncodeError(err, w)
	}
}

func printEncodeError(err error, w http.ResponseWriter) {
	log.Printf("Error encoding response: %v", err)
	http.Error(w, "Internal server error", http.StatusInternalServerError)
}
