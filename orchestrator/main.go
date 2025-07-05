package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

// OrderRequest is the input request
type OrderRequest struct {
	ID     string  `json:"id"`
	Amount float64 `json:"amount"`
}

// PaymentRequest for the payment service
type PaymentRequest struct {
	OrderID string `json:"orderID"`
}

// ShippingRequest for the shipping service
type ShippingRequest struct {
	OrderID string `json:"orderID"`
	Address string `json:"address"`
}

// callService sends JSON payload and returns the HTTP response.
func callService(url string, payload interface{}) (*http.Response, error) {
	b, _ := json.Marshal(payload)
	return http.Post(url, "application/json", bytes.NewReader(b))
}

const (
	warnCopy  = "Warning: io.Copy failed: %v"
	warnClose = "Warning: resp.Body.Close failed: %v"
)

// drainAndClose fully reads and closes the response body, logging any errors.
func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		log.Printf(warnCopy, err)
	}
	if err := resp.Body.Close(); err != nil {
		log.Printf(warnClose, err)
	}
}

// callAndCheck calls the given address with the payload and ensures the status code matches expected.
// It returns the raw response so the caller can drainAndClose it.
func callAndCheck(url string, payload interface{}, expectedStatus int) (*http.Response, error) {
	resp, err := callService(url, payload)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != expectedStatus {
		return resp, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	return resp, nil
}

// orchestrate manages the orchestrated saga flow with CORS and compensations
func orchestrate(w http.ResponseWriter, r *http.Request) {
	// CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.WriteHeader(http.StatusOK)
		return
	}

	// Parse input
	var req OrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// 1. Create Order
	createURL := "http://order-service:8081/orders"
	resp, err := callAndCheck(createURL, map[string]interface{}{"orderID": req.ID}, http.StatusCreated)
	if err != nil {
		http.Error(w, "order failed", http.StatusInternalServerError)
		return
	}
	drainAndClose(resp)

	// 2. Charge Payment
	payURL := "http://payment-service:8082/pay"
	resp, err = callAndCheck(payURL, PaymentRequest{OrderID: req.ID}, http.StatusOK)
	if err != nil {
		// compensation: cancel order
		if _, cancelErr := callService("http://order-service:8081/orders/"+req.ID+"/cancel", nil); cancelErr != nil {
			log.Printf("Compensation error: cancel order %s: %v", req.ID, cancelErr)
		}
		http.Error(w, "payment failed", http.StatusInternalServerError)
		return
	}
	drainAndClose(resp)

	// 3. Schedule Shipping
	shipURL := "http://shipping-service:8083/ship"
	resp, err = callAndCheck(shipURL, ShippingRequest{OrderID: req.ID, Address: "Default Address"}, http.StatusOK)
	if err != nil {
		// compensation: refund + cancel
		if _, refundErr := callService("http://payment-service:8082/pay/"+req.ID+"/refund", nil); refundErr != nil {
			log.Printf("Compensation error: refund payment %s: %v", req.ID, refundErr)
		}
		if _, cancelErr := callService("http://order-service:8081/orders/"+req.ID+"/cancel", nil); cancelErr != nil {
			log.Printf("Compensation error: cancel order %s: %v", req.ID, cancelErr)
		}
		http.Error(w, "shipping failed", http.StatusInternalServerError)
		return
	}
	drainAndClose(resp)

	// All OK
	w.WriteHeader(http.StatusOK)
}

func main() {
	http.HandleFunc("/saga", orchestrate)
	log.Println("Orchestrator listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
