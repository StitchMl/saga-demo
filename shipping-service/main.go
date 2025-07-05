package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
)

type ShipRequest struct {
	OrderID string `json:"orderID"`
	Address string `json:"address"`
}

var (
	shipments   = make(map[string]bool)
	shipmentsMu = sync.Mutex{}
)

func shipHandler(w http.ResponseWriter, r *http.Request) {
	var req ShipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	// We simulate an error if OrderID ends with “fail”.
	if strings.HasSuffix(req.OrderID, "fail") {
		http.Error(w, "shipping failed", http.StatusInternalServerError)
		return
	}

	// Register the shipment as successful
	shipmentsMu.Lock()
	shipments[req.OrderID] = true
	shipmentsMu.Unlock()

	w.WriteHeader(http.StatusOK)
}

func cancelShipHandler(w http.ResponseWriter, r *http.Request) {
	// URL: /ship/{orderID}/cancel-shipping
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) != 4 {
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	orderID := parts[2]

	shipmentsMu.Lock()
	defer shipmentsMu.Unlock()

	if _, ok := shipments[orderID]; !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Removes the shipment
	delete(shipments, orderID)
	w.WriteHeader(http.StatusOK)
}

func main() {
	http.HandleFunc("/ship", shipHandler)
	http.HandleFunc("/ship/", cancelShipHandler) // handles /ship/{orderID}/cancel-shipping

	log.Println("Shipping Service listening on :8083")
	log.Fatal(http.ListenAndServe(":8083", nil))
}
