package main

import (
	"encoding/json"
	"github.com/StitchMl/saga-demo/backend/common/payment_gateway"
	"log"
	"net/http"
	"os"
	"sync"
)

const (
	errorEncode = "Failed to encode response"
	errorBody   = "Invalid request body"
	errorMethod = "Method not allowed"
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
		OrderID, CustomerID string
		Amount              float64
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, errorBody, http.StatusBadRequest)
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
		http.Error(w, errorEncode, http.StatusBadRequest)
		return
	}

	transactionsDB.Data[req.OrderID] = "processed"
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Payment processed"})
}

// Manager to cancel a payment (offsetting)
func revertPaymentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, errorMethod, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		OrderID string `json:"order_id"`
		Reason  string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, errorBody, http.StatusBadRequest)
		return
	}

	transactionsDB.Lock()
	defer transactionsDB.Unlock()

	if transactionsDB.Data[req.OrderID] != "processed" {
		http.Error(w, "Payment not being processed locally or already cancelled", http.StatusBadRequest)
		return
	}

	gatewayStatus, gatewayErr := payment_gateway.RevertPayment(req.OrderID, req.Reason)
	if gatewayErr != nil || gatewayStatus != "success" {
		http.Error(w, "Payment reversal failed at gateway", http.StatusInternalServerError)
		return
	}

	transactionsDB.Data[req.OrderID] = "reverted"
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Payment cancelled"})
}
