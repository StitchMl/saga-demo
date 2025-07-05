package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// PaymentRequest represents the structure of the payload received for the payment.
type PaymentRequest struct {
	OrderID string  `json:"OrderID"`
	Amount  float64 `json:"Amount"`
}

// Event represents an event to be published by this service.
type Event struct {
	Type string      `json:"Type"`
	Data interface{} `json:"Data"`
}

// EventBusPayload is the structure of the events received by the Event Bus.
// Note: The Date field is of type json.RawMessage to allow lazy unmarshalling.
type EventBusPayload struct {
	Type string          `json:"Type"`
	Data json.RawMessage `json:"Data"`
}

var (
	// httpClient to make requests to the Event Bus with a timeout.
	httpClient = &http.Client{
		Timeout: 5 * time.Second,
	}
	paymentSimulator PaymentSimulator // Instance of an external payment simulator
)

// subscribeToOrderCreated subscribes this service to the OrderCreated event, with retry logic.
func subscribeToOrderCreated() {
	sub := map[string]string{
		"Type": "OrderCreated",
		"Url":  "http://payment-service-choreo:8082/pay",
	}
	payload, err := json.Marshal(sub)
	if err != nil {
		log.Fatalf("Error in the subscription payload marshal: %v", err) // Fatal if you can't even marshal
	}

	// Retry logic for subscribing to the Event Bus.
	maxRetries := 5
	retryDelay := 2 * time.Second
	for i := 0; i < maxRetries; i++ {
		log.Printf("Attempt to subscribe to OrderCreated event (attempt %d/%d)", i+1, maxRetries)
		resp, err := httpClient.Post("http://event-bus:8070/subscribe", "application/json", bytes.NewReader(payload))
		if err == nil && resp.StatusCode == http.StatusOK {
			log.Println("Successfully subscribed to the OrderCreated event.")
			// Close the body and handle the closure error
			if closeErr := resp.Body.Close(); closeErr != nil {
				log.Printf("Error closing response body after successful subscription: %v", closeErr)
			}
			return
		}

		if err != nil {
			log.Printf("Subscription attempt failed: %v. Retry in %v...", err, retryDelay)
		} else {
			log.Printf("Subscription attempt returned status %d. Retry in %v...", resp.StatusCode, retryDelay)
			// Chiudi il corpo e gestisci l'errore di chiusura
			if closeErr := resp.Body.Close(); closeErr != nil {
				log.Printf("Error closing response body after subscription failed: %v", closeErr)
			}
		}
		time.Sleep(retryDelay)
	}
	log.Fatalf("Failed to subscribe to OrderCreated event after %d attempts. Exit.", maxRetries)
}

// publishEvent sends an event to the Event Bus.
func publishEvent(eventType string, data interface{}) error {
	evt := Event{Type: eventType, Data: data}
	payload, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("error in marshalling the event payload: %v", err)
	}
	resp, err := httpClient.Post("http://event-bus:8070/publish", "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("POST to event bus failed: %v", err)
	}
	defer func() {
		if ckErr := resp.Body.Close(); ckErr != nil {
			log.Printf("Error in closing the body of the publish response: %v", ckErr)
		}
	}()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("publish returned non-OK status %d", resp.StatusCode)
	}
	return nil
}

// payHandler handles the /pay request, processing OrderCreated events.
func payHandler(w http.ResponseWriter, r *http.Request) {
	// Make sure that only POST requests are handled.
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var eventBusPayload EventBusPayload
	// Decodes the complete payload from the Event Bus.
	if err := json.NewDecoder(r.Body).Decode(&eventBusPayload); err != nil {
		http.Error(w, "Invalid event bus payload", http.StatusBadRequest)
		log.Printf("Error decoding the payload from the Event Bus: %v", err)
		return
	}

	// Check the type of event.
	if eventBusPayload.Type != "OrderCreated" {
		http.Error(w, "type of unexpected event", http.StatusBadRequest)
		log.Printf("Received unexpected event type: %s", eventBusPayload.Type)
		return
	}

	var req PaymentRequest
	// Decodes the 'Date' field (which is json.RawMessage) in the struct PaymentRequest.
	if err := json.Unmarshal(eventBusPayload.Data, &req); err != nil {
		http.Error(w, "Invalid OrderCreated event data", http.StatusBadRequest)
		log.Printf("Error in the unmarshal of OrderCreated Data in PaymentRequest: %v", err)
		return
	}

	log.Printf("Payment processing for OrderID: %s, Amount: %.2f (received from OrderCreated event)", req.OrderID, req.Amount)

	eventType := "PaymentSucceeded"
	// Use the external payment simulator.
	if err := paymentSimulator.ProcessPayment(req.OrderID, req.Amount); err != nil {
		eventType = "PaymentFailed"
		log.Printf("Payment processing failed for OrderID %s: %v", req.OrderID, err)
	} else {
		log.Printf("Payment successfully processed for OrderID: %s", req.OrderID)
	}

	// Publish the outcome event (PaymentSucceeded or PaymentFailed).
	if err := publishEvent(eventType, map[string]string{"OrderID": req.OrderID}); err != nil {
		log.Printf("Publication %s failed: %v", eventType, err)
		http.Error(w, "event bus error", http.StatusInternalServerError)
		return
	}

	// Reply to the Event Bus indicating that the event was received and processed successfully.
	w.WriteHeader(http.StatusOK)
}

func main() {
	paymentSimulator = NewExternalPaymentSimulator() // Initialize Payment Simulator

	// Subscribe to the Event Bus at start-up, with retry logic.
	subscribeToOrderCreated()

	http.HandleFunc("/pay", payHandler)

	log.Println("Payment Service (choreo) listening on port :8082")
	if err := http.ListenAndServe(":8082", nil); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
