package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	inventorydb "github.com/StitchMl/saga-demo/common/data_store"
	events "github.com/StitchMl/saga-demo/common/types"
)

const (
	contentTypeJSON = "application/json"
	contentType     = "Content-Type"
)

func main() {
	port := os.Getenv("INVENTORY_SERVICE_PORT")
	if port == "" {
		port = "8082"
	}
	inventorydb.InitDB()
	http.HandleFunc("/reserve", reserveInventoryHandler)
	http.HandleFunc("/cancel_reservation", cancelReservationHandler)
	http.HandleFunc("/catalog", catalogHandler)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// getEnvOrFail retrieves environment variables or exits if not found
func catalogHandler(w http.ResponseWriter, _ *http.Request) {
	type Product struct {
		ID          string  `json:"id"`
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Price       float64 `json:"price"`
		Available   int     `json:"available"`
	}

	// In this example, collect data from DataStore in-memory
	inventorydb.DB.Inventory.RLock()
	inventorydb.DB.Prices.RLock()
	defer inventorydb.DB.Inventory.RUnlock()
	defer inventorydb.DB.Prices.RUnlock()

	list := make([]Product, 0, len(inventorydb.DB.Inventory.Data))
	for id, qty := range inventorydb.DB.Inventory.Data {
		price := inventorydb.DB.Prices.Data[id]
		list = append(list, Product{
			ID: id, Name: id, Description: "",
			Price: price, Available: qty,
		})
	}
	w.Header().Set(contentType, contentTypeJSON)
	_ = json.NewEncoder(w).Encode(list)
}

// Manager to reserve products
func reserveInventoryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req events.InventoryRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	inventorydb.DB.Inventory.Lock()
	defer inventorydb.DB.Inventory.Unlock()

	for _, item := range req.Items {
		available, ok := inventorydb.DB.Inventory.Data[item.ProductID]
		if !ok || available < item.Quantity {
			http.Error(w, "Insufficient quantity or no product", http.StatusBadRequest)
			return
		}
		inventorydb.DB.Inventory.Data[item.ProductID] -= item.Quantity
	}

	w.Header().Set(contentType, contentTypeJSON)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Inventory reserved"})
}

// Manager to cancel a reserve of products (compensation)
func cancelReservationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req events.InventoryRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	inventorydb.DB.Inventory.Lock()
	defer inventorydb.DB.Inventory.Unlock()

	for _, item := range req.Items {
		inventorydb.DB.Inventory.Data[item.ProductID] += item.Quantity
	}

	w.Header().Set(contentType, contentTypeJSON)
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Reserve canceled and inventory restored",
	}); err != nil {
		printEncodeError(err, w)
	}
}

func printEncodeError(err error, w http.ResponseWriter) {
	log.Printf("Error encoding response: %v", err)
	http.Error(w, "Internal server error", http.StatusInternalServerError)
}
