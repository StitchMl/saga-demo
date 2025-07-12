package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"io"
	"log"
	"net/http"
	"sync"
)

// ShipRequest defines the structure of the shipping request.
type ShipRequest struct {
	OrderID string `json:"orderID"`
	Address string `json:"address"`
}

var (
	// shipments is a map that keeps track of shipped orders.
	shipments   = make(map[string]bool)
	shipmentsMu sync.Mutex // Mutex to protect access to map shipments.

	// shippingSimulator is the instance of the external shipping simulator.
	shippingSimulator ShippingSimulator
)

// shipHandler handles shipping requests.
func shipHandler(w http.ResponseWriter, r *http.Request) {
	// Ensures that the HTTP method is POST.
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ShipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	// *** HERE WE CALL THE EXTERNAL SHIPPING SERVICE ***
	log.Printf("Calling external shipping service for OrderID: %s, Address: %s", req.OrderID, req.Address)
	if err := shippingSimulator.ScheduleShipment(req.OrderID, req.Address); err != nil {
		log.Printf("External shipping service failed for OrderID %s: %v", req.OrderID, err)
		http.Error(w, fmt.Sprintf("shipping processing failed: %v", err), http.StatusInternalServerError)
		return
	}
	log.Printf("External shipping service succeeded for OrderID: %s", req.OrderID)

	// It only registers the shipment as successful after the success of the simulator.
	shipmentsMu.Lock()
	shipments[req.OrderID] = true
	shipmentsMu.Unlock()

	w.WriteHeader(http.StatusOK)
}

// cancelShipHandler handles shipment cancellation requests.
func cancelShipHandler(w http.ResponseWriter, r *http.Request) {
	// Ensures that the HTTP method is POST.
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	vars := mux.Vars(r) // Gets variables from the path via mux.
	orderID := vars["orderID"]

	shipmentsMu.Lock()
	defer shipmentsMu.Unlock() // It ensures that the mutex is released.

	if _, ok := shipments[orderID]; !ok {
		http.Error(w, "shipment not found for cancellation", http.StatusNotFound)
		return
	}

	// Removes the shipment.
	delete(shipments, orderID)
	w.WriteHeader(http.StatusNoContent) // 204 No Content is more correct for successful deletions.
	log.Printf("Shipment cancelled for OrderID: %s", orderID)
}

// healthCheckHandler responds with 200 OK for healthchecks.
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	if _, err := io.WriteString(w, "OK"); err != nil {
		log.Printf("Warning: Error writing health check response: %v", err)
	}
}

func main() {
	// Initializes the external shipping simulator at start-up.
	shippingSimulator = NewExternalShippingSimulator()

	router := mux.NewRouter()

	// Route for shipment (POST /ship).
	router.HandleFunc("/ship", shipHandler).Methods("POST")

	// Route for shipment cancellation (POST /ship/{orderID}/cancel-shipping).
	// Uses a path variable 'orderID'.
	router.HandleFunc("/ship/{orderID}/cancel-shipping", cancelShipHandler).Methods("POST")

	// Register the health check handler with the mux router
	router.HandleFunc("/health", healthCheckHandler).Methods("GET")

	log.Println("Shipping Service listening on :8083")
	log.Fatal(http.ListenAndServe(":8083", router)) // Use the router.
}
