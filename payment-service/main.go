package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
)

type PayRequest struct {
	OrderID string  `json:"orderID"`
	Amount  float64 `json:"amount"`
}

var (
	payments   = make(map[string]bool)
	paymentsMu = sync.Mutex{}
)

func payHandler(w http.ResponseWriter, r *http.Request) {
	var req PayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	// Simuliamo un errore se OrderID termina con "fail"
	if strings.HasSuffix(req.OrderID, "fail") {
		http.Error(w, "payment failed", http.StatusInternalServerError)
		return
	}

	// Registra il pagamento come riuscito
	paymentsMu.Lock()
	payments[req.OrderID] = true
	paymentsMu.Unlock()

	w.WriteHeader(http.StatusOK)
}

func refundHandler(w http.ResponseWriter, r *http.Request) {
	// URL: /pay/{orderID}/refund
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) != 4 {
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	orderID := parts[2]

	paymentsMu.Lock()
	defer paymentsMu.Unlock()

	if _, ok := payments[orderID]; !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Rimuove il pagamento
	delete(payments, orderID)
	w.WriteHeader(http.StatusOK)
}

func main() {
	http.HandleFunc("/pay", payHandler)
	http.HandleFunc("/pay/", refundHandler) // gestisce /pay/{orderID}/refund

	log.Println("Payment Service listening on :8082")
	log.Fatal(http.ListenAndServe(":8082", nil))
}
