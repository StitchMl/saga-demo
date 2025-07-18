package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/StitchMl/saga-demo/common/payment_gateway"
)

const (
	errorEncode     = "Failed to encode response"
	errorBody       = "Invalid request body"
	errorMethod     = "Method not allowed"
	contentTypeJSON = "application/json"
	contentType     = "Content-Type"
)

// In-memory database for payment transactions (local record of the Payment Service)
var transactionsDB = struct {
	sync.RWMutex
	Data map[string]string // Map OrderID to transaction status (for example, “pending”, “processed”, “reverted”, “failed”)
}{Data: make(map[string]string)}

func main() {
	port := os.Getenv("PAYMENT_SERVICE_PORT")
	if port == "" {
		log.Printf("PAYMENT_SERVICE_PORT not set! Must be set to run the service.")
	}
	http.HandleFunc("/process", processPaymentHandler)
	http.HandleFunc("/revert", revertPaymentHandler)
	log.Printf("Payment Service started on the port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// Manager to process a payment
func processPaymentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, errorMethod, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		OrderID    string  `json:"order_id"`
		CustomerID string  `json:"customer_id"`
		Amount     float64 `json:"amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// rispondi SEMPRE in JSON: l’Orchestrator si aspetta JSON
		w.Header().Set(contentType, contentTypeJSON)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": errorBody,
		})
		return
	}

	transactionsDB.Lock()
	transactionsDB.Data[req.OrderID] = "pending"
	transactionsDB.Unlock()

	status, err := payment_gateway.ProcessPayment(req.OrderID, req.CustomerID, req.Amount)

	transactionsDB.Lock()
	defer transactionsDB.Unlock()
	if err != nil || status != "success" {
		transactionsDB.Data[req.OrderID] = "failed"
		w.Header().Set(contentType, contentTypeJSON)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": errorEncode,
		})
		return
	}

	transactionsDB.Data[req.OrderID] = "processed"
	w.Header().Set(contentType, contentTypeJSON)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Payment processed"})
}

// Manager to cancel a payment (offsetting)
func revertPaymentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, errorMethod, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		OrderID    string  `json:"order_id"`
		CustomerID string  `json:"customer_id"`
		Amount     float64 `json:"amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set(contentType, contentTypeJSON)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": errorBody,
		})
		return
	}

	transactionsDB.Lock()
	defer transactionsDB.Unlock()

	if transactionsDB.Data[req.OrderID] != "processed" {
		http.Error(w, "Payment not being processed locally or already cancelled", http.StatusBadRequest)
		return
	}

	gatewayStatus, gatewayErr := payment_gateway.RevertPayment(req.OrderID, "Reverting payment for order "+req.OrderID)
	if gatewayErr != nil || gatewayStatus != "success" {
		http.Error(w, "Payment reversal failed at gateway", http.StatusInternalServerError)
		return
	}

	transactionsDB.Data[req.OrderID] = "reverted"
	w.Header().Set(contentType, contentTypeJSON)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Payment cancelled"})
}
