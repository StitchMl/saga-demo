package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

// PaymentRequest represents the structure of the received payload
type PaymentRequest struct {
	OrderID string  `json:"OrderID"`
	Amount  float64 `json:"Amount"`
}

// Event represents an event to be published
type Event struct {
	Type string      `json:"Type"`
	Data interface{} `json:"Data"`
}

// mustPost sends a POST and handles errors + body closure
func mustPost(url string, payload []byte, desc string) {
	resp, err := http.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		log.Fatalf("%s POST to %s failed: %v", desc, url, err)
	}
	defer func() {
		if ckErr := resp.Body.Close(); ckErr != nil {
			log.Printf("%s close error: %v", desc, ckErr)
		}
	}()
	if resp.StatusCode >= 300 {
		log.Fatalf("%s POST to %s returned status %d", desc, url, resp.StatusCode)
	}
}

// subscribeToOrderCreated subscribes this service to OrderCreated
func subscribeToOrderCreated() {
	sub := map[string]string{
		"Type": "OrderCreated",
		"Url":  "http://payment-service-choreo:8082/pay",
	}
	payload, err := json.Marshal(sub)
	if err != nil {
		log.Fatalf("marshal subscription payload: %v", err)
	}
	mustPost("http://event-bus:8070/subscribe", payload, "Subscription")
}

// publishEvent send an event to the Event Bus
func publishEvent(eventType string, data interface{}) error {
	evt := Event{Type: eventType, Data: data}
	payload, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	resp, err := http.Post("http://event-bus:8070/publish", "application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer func() {
		if ckErr := resp.Body.Close(); ckErr != nil {
			log.Printf("publish close error: %v", ckErr)
		}
	}()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("publish returned %d", resp.StatusCode)
	}
	return nil
}

// payHandler handles the /pay request
func payHandler(w http.ResponseWriter, r *http.Request) {
	var req PaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid input", http.StatusBadRequest)
		return
	}
	eventType := "PaymentSucceeded"
	if req.OrderID == "fail" {
		eventType = "PaymentFailed"
	}

	if err := publishEvent(eventType, map[string]string{"OrderID": req.OrderID}); err != nil {
		log.Printf("Publish %s failed: %v", eventType, err)
		http.Error(w, "event bus error", http.StatusInternalServerError)
		return
	}

	if eventType == "PaymentFailed" {
		http.Error(w, "payment error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func main() {
	subscribeToOrderCreated()

	http.HandleFunc("/pay", payHandler)

	log.Println("Payment Service (choreo) listening on :8082")
	if err := http.ListenAndServe(":8082", nil); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
