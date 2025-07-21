package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/StitchMl/saga-demo/common/payment_gateway"
	events "github.com/StitchMl/saga-demo/common/types"
)

const (
	errorBody       = "Invalid request body"
	errorMethod     = "Method not allowed"
	contentTypeJSON = "application/json"
	contentType     = "Content-Type"
)

var paymentAmountLimit float64

// In-memory database for payment transactions (local record of the Payment Service)
var transactionsDB = struct {
	sync.RWMutex
	Data map[string]string // Map OrderID to transaction status (for example, “pending”, “processed”, “reverted”, “failed”)
}{Data: make(map[string]string)}

func main() {
	port := os.Getenv("PAYMENT_SERVICE_PORT")
	if port == "" {
		log.Fatal("PAYMENT_SERVICE_PORT environment variable not set.")
	}

	limitStr := os.Getenv("PAYMENT_AMOUNT_LIMIT")
	if limitStr == "" {
		log.Fatal("PAYMENT_AMOUNT_LIMIT not set")
	}
	var err error
	paymentAmountLimit, err = strconv.ParseFloat(limitStr, 64)
	if err != nil {
		log.Fatalf("Invalid PAYMENT_AMOUNT_LIMIT: %v", err)
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

	var req events.PaymentPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// ALWAYS respond in JSON: the Orchestrator expects JSON
		w.Header().Set(contentType, contentTypeJSON)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": errorBody,
		})
		return
	}

	// Check payment limit
	if req.Amount > paymentAmountLimit {
		w.Header().Set(contentType, contentTypeJSON)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": fmt.Sprintf("Payment processing failed: amount %.2f exceeds limit", req.Amount),
		})
		return
	}

	transactionsDB.Lock()
	transactionsDB.Data[req.OrderID] = "pending"
	transactionsDB.Unlock()

	err := payment_gateway.ProcessPayment(req.OrderID, req.CustomerID, req.Amount)

	transactionsDB.Lock()
	defer transactionsDB.Unlock()
	if err != nil {
		transactionsDB.Data[req.OrderID] = "failed"
		w.Header().Set(contentType, contentTypeJSON)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "error",
			"message": "Payment processing failed" + func() string {
				if err != nil {
					return ": " + err.Error()
				} else {
					return ""
				}
			}(),
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
		OrderID string `json:"order_id"`
		Reason  string `json:"reason"`
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
		// If the payment has not been processed, we consider the compensation a success.
		log.Printf("Payment for order %s was not processed, no need to revert.", req.OrderID)
		w.Header().Set(contentType, contentTypeJSON)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Payment not processed, no action taken"})
		return
	}

	gatewayErr := payment_gateway.RevertPayment(req.OrderID, req.Reason)
	if gatewayErr != nil {
		log.Printf("Payment reversal failed at gateway for order %s: %v", req.OrderID, gatewayErr)
		http.Error(w, "Payment reversal failed at gateway", http.StatusInternalServerError)
		return
	}

	transactionsDB.Data[req.OrderID] = "reverted"
	log.Printf("Reverted payment for order %s", req.OrderID)
	w.Header().Set(contentType, contentTypeJSON)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Payment reverted"})
}
