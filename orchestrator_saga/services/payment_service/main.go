package main

import (
	"encoding/json"
	"github.com/StitchMl/saga-demo/common/payment_gateway"
	"log"
	"net/http"
	"sync"
	"time"
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
	http.HandleFunc("/process", processPaymentHandler)
	http.HandleFunc("/revert", revertPaymentHandler)

	log.Println("Payment Service started on port 8083")
	log.Fatal(http.ListenAndServe(":8083", nil))
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
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, errorBody, http.StatusBadRequest)
		return
	}

	// Simulates a delay for the payment service's internal processing before contacting the gateway.
	time.Sleep(50 * time.Millisecond)

	transactionsDB.Lock()
	// Set initial status to pending before contacting the gateway
	transactionsDB.Data[req.OrderID] = "pending"
	transactionsDB.Unlock() // Unlock after the initial status update

	log.Printf("Payment Service: Initiating payment for Order %s with Gateway.", req.OrderID)

	// Interaction with the simulated gateway
	gatewayStatus, gatewayErr := payment_gateway.ProcessPayment(req.OrderID, req.CustomerID, req.Amount) // Calling the simulated gateway

	transactionsDB.Lock() // Re-lock to update status based on gateway response
	defer transactionsDB.Unlock()

	if gatewayErr != nil || gatewayStatus != "success" {
		log.Printf("Payment Service: Payment for Order %s failed at Gateway. Error: %v, Gateway Status: %s", req.OrderID, gatewayErr, gatewayStatus)
		transactionsDB.Data[req.OrderID] = "failed" // Update local status to fail
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "failure", "message": "Payment failed at gateway"}); err != nil {
			http.Error(w, errorEncode, http.StatusInternalServerError)
		}
		return
	}

	// If gateway reports success
	log.Printf("Payment Service: Payment for Order %s successfully processed by Gateway.", req.OrderID)
	transactionsDB.Data[req.OrderID] = "processed" // Update local status to processed

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Payment processed"}); err != nil {
		http.Error(w, errorEncode, http.StatusInternalServerError)
	}
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
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, errorBody, http.StatusBadRequest)
		return
	}

	transactionsDB.Lock()
	defer transactionsDB.Unlock()

	currentLocalStatus := transactionsDB.Data[req.OrderID]

	if currentLocalStatus != "processed" {
		log.Printf("Payment Service: Attempt to cancel payment for Order %s, but local status is not 'processed' (current status: %s)", req.OrderID, currentLocalStatus)
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "failure", "message": "Payment not being processed locally or already cancelled"}); err != nil {
			http.Error(w, errorEncode, http.StatusInternalServerError)
		}
		return
	}

	log.Printf("Payment Service: Initiating payment reversal for Order %s with Gateway.", req.OrderID)

	// Interaction with the simulated gateway
	gatewayStatus, gatewayErr := payment_gateway.RevertPayment(req.OrderID, req.Reason) // Calling the simulated gateway

	if gatewayErr != nil || gatewayStatus != "success" {
		log.Printf("Payment Service: Payment reversal for Order %s failed at Gateway. Error: %v, Gateway Status: %s", req.OrderID, gatewayErr, gatewayStatus)
		// Consider whether the local state should remain 'processed' or go into 'reversal_failed'.
		w.WriteHeader(http.StatusInternalServerError) // Or BadGateway, depending on the error
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "failure", "message": "Payment reversal failed at gateway"}); err != nil {
			http.Error(w, errorEncode, http.StatusInternalServerError)
		}
		return
	}

	// If gateway reports success
	transactionsDB.Data[req.OrderID] = "reverted" // Update local status to revert
	log.Printf("Payment Service: Payment for Order %s successfully reverted by Gateway (reason: %s).", req.OrderID, req.Reason)
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Payment cancelled"}); err != nil {
		http.Error(w, errorEncode, http.StatusInternalServerError)
	}
}
