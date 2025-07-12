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

// PayRequest defines the structure of the payment request.
type PayRequest struct {
	OrderID string  `json:"orderID"`
	Amount  float64 `json:"amount"`
}

var (
	// payments is a map that keeps track of orders for which a payment has been made.
	payments   = make(map[string]bool)
	paymentsMu sync.Mutex // Mutex per proteggere l'accesso alla mappa payments.

	// paymentSimulator is the instance of the external payment simulator.
	paymentSimulator PaymentSimulator
)

// payHandler handles payment requests.
func payHandler(w http.ResponseWriter, r *http.Request) {
	// Ensures that the HTTP method is POST.
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req PayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	// 1. Validation of the Amount.
	if req.Amount <= 0 {
		http.Error(w, "amount must be positive", http.StatusBadRequest)
		return
	}

	paymentsMu.Lock()
	defer paymentsMu.Unlock() // It ensures that the mutex is released.

	// 2. Duplicate payment management.
	if _, ok := payments[req.OrderID]; ok {
		http.Error(w, "payment already exists for this order", http.StatusConflict) // 409 Conflict
		return
	}

	// *** HERE WE CALL THE EXTERNAL PAYMENT SERVICE ***
	log.Printf("Calling external payment service for OrderID: %s, Amount: %.2f", req.OrderID, req.Amount)
	if err := paymentSimulator.ProcessPayment(req.OrderID, req.Amount); err != nil {
		log.Printf("External payment service failed for OrderID %s: %v", req.OrderID, err)
		http.Error(w, fmt.Sprintf("payment processing failed: %v", err), http.StatusInternalServerError)
		return
	}
	log.Printf("External payment service succeeded for OrderID: %s", req.OrderID)

	// It records the payment as successful only after the success of the simulator.
	payments[req.OrderID] = true

	w.WriteHeader(http.StatusOK)
}

// refundHandler handles refund requests.
func refundHandler(w http.ResponseWriter, r *http.Request) {
	// Ensures that the HTTP method is POST.
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	vars := mux.Vars(r) // Gets variables from the path via mux.
	orderID := vars["orderID"]

	paymentsMu.Lock()
	defer paymentsMu.Unlock() // It ensures that the mutex is released.

	if _, ok := payments[orderID]; !ok {
		http.Error(w, "payment not found for refund", http.StatusNotFound)
		return
	}

	// Removes the payment.
	delete(payments, orderID)
	w.WriteHeader(http.StatusOK) // 200 OK is acceptable for reimbursement.
	log.Printf("Payment refunded for OrderID: %s", orderID)
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
	// Initializes the external payment simulator at start-up.
	paymentSimulator = NewExternalPaymentSimulator()

	router := mux.NewRouter()

	// Route for payment (POST /pay).
	router.HandleFunc("/pay", payHandler).Methods("POST")

	// Route for reimbursement (POST /pay/{orderID}/refund).
	// Uses a path variable 'orderID'.
	router.HandleFunc("/pay/{orderID}/refund", refundHandler).Methods("POST")

	// Register the health check handler with the mux router
	router.HandleFunc("/health", healthCheckHandler).Methods("GET")

	log.Println("Payment Service listening on :8082")
	log.Fatal(http.ListenAndServe(":8082", router)) // Use the router.
}
