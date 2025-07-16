package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	inventorydb "github.com/StitchMl/saga-demo/backend/common/data_store"
	"github.com/StitchMl/saga-demo/backend/common/types"
)

func main() {
	port := os.Getenv("INVENTORY_SERVICE_PORT")
	if port == "" {
		port = "8082"
	}
	inventorydb.InitDB()
	http.HandleFunc("/reserve", reserveInventoryHandler)
	http.HandleFunc("/cancel_reservation", cancelReservationHandler)
	log.Fatal(http.ListenAndServe(":"+port, nil))
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Inventory reserved"})
}

// Manager to cancel a reserve of products (compensation)
func cancelReservationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Metodo non consentito", http.StatusMethodNotAllowed)
		return
	}

	var req events.InventoryRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Richiesta non valida", http.StatusBadRequest)
		return
	}

	inventorydb.DB.Inventory.Lock()
	defer inventorydb.DB.Inventory.Unlock()

	for _, item := range req.Items {
		inventorydb.DB.Inventory.Data[item.ProductID] += item.Quantity
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Riserva annullata e inventario ripristinato",
	}); err != nil {
		printEncodeError(err, w)
	}
}

func printEncodeError(err error, w http.ResponseWriter) {
	log.Printf("Error encoding response: %v", err)
	http.Error(w, "Internal server error", http.StatusInternalServerError)
}
