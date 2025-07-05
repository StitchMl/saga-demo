package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// OrderRequest is the input request for the order.
type OrderRequest struct {
	ID     string  `json:"id"`
	Amount float64 `json:"amount"`
}

// PaymentRequest for the payment service.
type PaymentRequest struct {
	OrderID string `json:"orderID"`
}

// ShippingRequest for the shipping service.
type ShippingRequest struct {
	OrderID string `json:"orderID"`
	Address string `json:"address"`
}

// OrderResponse is the response expected from the order service after creation.
type OrderResponse struct {
	OrderID string `json:"orderID"`
}

const (
	warnCopy  = "Warning: io.Copy failed: %v"
	warnClose = "Warning: resp.Body.Close failed: %v"

	// Service URLs, moved to constants for flexibility.
	orderServiceURL    = "http://order-service:8081"
	paymentServiceURL  = "http://payment-service:8082"
	shippingServiceURL = "http://shipping-service:8083"
)

// httpClient with a timeout to prevent indefinite blocking.
var httpClient = &http.Client{
	Timeout: 10 * time.Second, // Timeout for all HTTP operations.
}

// callService sends a JSON payload and returns the HTTP response.
// Includes error handling for json.Marshal and context support.
func callService(ctx context.Context, url string, payload interface{}) (*http.Response, error) {
	var b []byte
	var err error
	if payload != nil {
		b, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payload: %w", err) // Handles the marshalling error.
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	return httpClient.Do(req)
}

// drainAndClose reads completely and closes the response body to avoid connection losses.
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

// callAndCheck calls the address given with the payload and checks that the status code matches the expected one.
// Returns the raw response so that the caller can close it.
func callAndCheck(ctx context.Context, url string, payload interface{}, expectedStatus int) (*http.Response, error) {
	resp, err := callService(ctx, url, payload)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != expectedStatus {
		return resp, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	return resp, nil
}

// performOrderCreation handles the order creation step.
func performOrderCreation(ctx context.Context, reqID string) (string, error) {
	createURL := orderServiceURL + "/orders"
	resp, err := callService(ctx, createURL, map[string]interface{}{"orderID": reqID})
	if err != nil {
		return "", fmt.Errorf("order creation request failed: %w", err)
	}
	defer drainAndClose(resp)

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("order creation unexpected status: %d", resp.StatusCode)
	}

	var orderResp OrderResponse
	if err := json.NewDecoder(resp.Body).Decode(&orderResp); err != nil {
		return "", fmt.Errorf("failed to decode order creation response: %w", err)
	}
	if orderResp.OrderID == "" {
		return "", fmt.Errorf("order service did not return an order ID")
	}
	return orderResp.OrderID, nil
}

// performPaymentCharge handles the debit step of the payment.
func performPaymentCharge(ctx context.Context, orderID string) error {
	payURL := paymentServiceURL + "/pay"
	resp, err := callAndCheck(ctx, payURL, PaymentRequest{OrderID: orderID}, http.StatusOK)
	if err != nil {
		return fmt.Errorf("payment charge failed: %w", err)
	}
	drainAndClose(resp)
	return nil
}

// performShippingSchedule manages the scheduling step of the shipment.
func performShippingSchedule(ctx context.Context, orderID string) error {
	shipURL := shippingServiceURL + "/ship"
	resp, err := callAndCheck(ctx, shipURL, ShippingRequest{OrderID: orderID, Address: "Default Address"}, http.StatusOK)
	if err != nil {
		return fmt.Errorf("shipping schedule failed: %w", err)
	}
	drainAndClose(resp)
	return nil
}

// handlePaymentCompensation performs compensation if payment failure.
func handlePaymentCompensation(ctx context.Context, orderID string) {
	cancelOrderResp, cancelErr := callService(ctx, orderServiceURL+"/orders/"+orderID+"/cancel", nil)
	if cancelErr != nil {
		log.Printf("Compensation error: cancel order %s: %v", orderID, cancelErr)
	}
	drainAndClose(cancelOrderResp)
}

// handleShippingCompensation performs compensation if of shipment failure.
func handleShippingCompensation(ctx context.Context, orderID string) {
	refundResp, refundErr := callService(ctx, paymentServiceURL+"/pay/"+orderID+"/refund", nil)
	if refundErr != nil {
		log.Printf("Compensation error: refund payment %s: %v", orderID, refundErr)
	}
	drainAndClose(refundResp)

	cancelOrderResp, cancelErr := callService(ctx, orderServiceURL+"/orders/"+orderID+"/cancel", nil)
	if cancelErr != nil {
		log.Printf("Compensation error: cancel order %s: %v", orderID, cancelErr)
	}
	drainAndClose(cancelOrderResp)

	// Compensation for shipping (cancellation).
	cancelShippingResp, cancelShippingErr := callService(ctx, shippingServiceURL+"/ship/"+orderID+"/cancel-shipping", nil)
	if cancelShippingErr != nil {
		log.Printf("Compensation error: cancel shipping %s: %v", orderID, cancelShippingErr)
	}
	drainAndClose(cancelShippingResp)
}

// orchestrate saga flow with CORS and compensations.
func orchestrate(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// CORS. management.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.WriteHeader(http.StatusOK)
		return
	}

	// Parses the incoming request.
	var req OrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// 1. Order creation.
	orderID, err := performOrderCreation(ctx, req.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("order creation failed: %v", err), http.StatusInternalServerError)
		return
	}

	// 2. Debit payment.
	err = performPaymentCharge(ctx, orderID)
	if err != nil {
		handlePaymentCompensation(ctx, orderID)
		http.Error(w, fmt.Sprintf("payment failed: %v", err), http.StatusInternalServerError)
		return
	}

	// 3. Shipment scheduling.
	err = performShippingSchedule(ctx, orderID)
	if err != nil {
		handleShippingCompensation(ctx, orderID)
		http.Error(w, fmt.Sprintf("shipping failed: %v", err), http.StatusInternalServerError)
		return
	}

	// All steps have been successfully completed.
	w.WriteHeader(http.StatusOK)
}

func main() {
	http.HandleFunc("/saga", orchestrate)
	log.Println("Orchestrator listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
