package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	events "github.com/StitchMl/saga-demo/common/types"
)

const (
	contentTypeJSON       = "application/json"
	contentType           = "Content-Type"
	errorMethodNotAllowed = "Metodo non consentito"
)

// ProductsDB simula un database di prodotti.
var ProductsDB = struct {
	sync.RWMutex
	Data map[string]events.Product
}{Data: make(map[string]events.Product)}

func initDB() {
	ProductsDB.Lock()
	defer ProductsDB.Unlock()

	ProductsDB.Data = map[string]events.Product{
		"prod-1": {ID: "laptop-pro", Name: "Laptop Pro", Description: "A powerful laptop for professionals.", Price: 1299.99, Available: 100, ImageURL: "https://m.media-amazon.com/images/I/61UcV2bDnoL._AC_SL1500_.jpg"},
		"prod-2": {ID: "mouse-wireless", Name: "Mouse Wireless", Description: "Ergonomic and precise mouse.", Price: 49.50, Available: 50, ImageURL: "https://m.media-amazon.com/images/I/711bP+FjSQL._AC_SL1500_.jpg"},
		"prod-3": {ID: "mechanical-keyboard", Name: "Mechanical Keyboard", Description: "Keyboard with mechanical switches for gaming.", Price: 120.00, Available: 200, ImageURL: "https://m.media-amazon.com/images/I/71kq6u7NA4L._AC_SL1500_.jpg"},
	}
	log.Println("[ServiceInventory] In-memory database initialized.")
}

func main() {
	port := os.Getenv("INVENTORY_SERVICE_PORT")
	if port == "" {
		log.Fatal("INVENTORY_SERVICE_PORT environment variable not set.")
	}
	initDB()
	http.HandleFunc("/reserve", reserveInventoryHandler)
	http.HandleFunc("/cancel_reservation", cancelReservationHandler)
	http.HandleFunc("/catalog", catalogHandler)
	http.HandleFunc("/get_price", getPriceHandler) // Nuovo endpoint per i prezzi
	log.Printf("Servizio Inventario avviato sulla porta %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// getPriceHandler returns the price of a single product.
func getPriceHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, errorMethodNotAllowed, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ProductID string `json:"product_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ProductsDB.RLock()
	product, ok := ProductsDB.Data[req.ProductID]
	ProductsDB.RUnlock()

	if !ok {
		http.Error(w, "Product not found", http.StatusNotFound)
		return
	}

	w.Header().Set(contentType, contentTypeJSON)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"product_id": req.ProductID,
		"price":      fmt.Sprintf("%.2f", product.Price),
		"status":     "success",
	})
}

// catalogHandler manages requests to get the product catalog.
func catalogHandler(w http.ResponseWriter, _ *http.Request) {
	ProductsDB.RLock()
	defer ProductsDB.RUnlock()

	list := make([]events.Product, 0, len(ProductsDB.Data))
	for _, p := range ProductsDB.Data {
		list = append(list, p)
	}
	w.Header().Set(contentType, contentTypeJSON)
	_ = json.NewEncoder(w).Encode(list)
}

// reserveInventoryHandler manages product reservation.
func reserveInventoryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, errorMethodNotAllowed, http.StatusMethodNotAllowed)
		return
	}

	var req events.InventoryRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ProductsDB.Lock()
	defer ProductsDB.Unlock()

	// Check availability before making changes
	for _, item := range req.Items {
		product, ok := ProductsDB.Data[item.ProductID]
		if !ok || product.Available < item.Quantity {
			w.Header().Set(contentType, contentTypeJSON)
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status":  "error",
				"message": "Insufficient quantity or product not found for " + item.ProductID,
			})
			return
		}
	}

	// Book articles
	for _, item := range req.Items {
		product := ProductsDB.Data[item.ProductID]
		product.Available -= item.Quantity
		ProductsDB.Data[item.ProductID] = product
	}

	log.Printf("Inventory booked for Order %s", req.OrderID)
	w.Header().Set(contentType, contentTypeJSON)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Booked inventory"})
}

// cancelReservationHandler manages the cancellation of a reservation (compensation).
func cancelReservationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, errorMethodNotAllowed, http.StatusMethodNotAllowed)
		return
	}

	var req events.InventoryRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	ProductsDB.Lock()
	defer ProductsDB.Unlock()

	for _, item := range req.Items {
		product := ProductsDB.Data[item.ProductID]
		product.Available += item.Quantity
		ProductsDB.Data[item.ProductID] = product
	}

	log.Printf("Canceled inventory reservation for Order %s", req.OrderID)
	w.Header().Set(contentType, contentTypeJSON)
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Reservation canceled and inventory restored",
	}); err != nil {
		printEncodeError(err, w)
	}
}

func printEncodeError(err error, w http.ResponseWriter) {
	log.Printf("Error in response coding: %v", err)
	http.Error(w, "Internal server error", http.StatusInternalServerError)
}
