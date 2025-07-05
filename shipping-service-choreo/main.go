package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// ShippingRequest represents the incoming payload for shipping.
type ShippingRequest struct {
	OrderID string `json:"OrderID"`
}

// Event represents an event to be published by this service.
type Event struct {
	Type string      `json:"Type"`
	Data interface{} `json:"Data"`
}

// EventBusPayload is the structure of the events received by the Event Bus.
// The Date field is of type json.RawMessage to allow lazy unmarshalling.
type EventBusPayload struct {
	Type string          `json:"Type"`
	Data json.RawMessage `json:"Data"`
}

var (
	// httpClient to make requests to the Event Bus with a timeout.
	httpClient = &http.Client{
		Timeout: 5 * time.Second,
	}
	shippingSimulator ShippingSimulator // Instance of the external shipping simulator
)

// subscribeToPaymentSucceeded subscribes this service to the PaymentSucceeded event, with retry logic.
func subscribeToPaymentSucceeded() {
	sub := map[string]string{
		"Type": "PaymentSucceeded",
		"Url":  "http://shipping-service-choreo:8083/ship",
	}
	payload, err := json.Marshal(sub)
	if err != nil {
		log.Fatalf("Error in the subscription payload marshal: %v", err) // Fatal if you can't even marshal
	}

	// Retry logic for subscribing to the Event Bus.
	maxRetries := 5
	retryDelay := 2 * time.Second
	for i := 0; i < maxRetries; i++ {
		log.Printf("Attempt to subscribe to the PaymentSucceeded event (attempt %d/%d)", i+1, maxRetries)
		resp, err := httpClient.Post("http://event-bus:8070/subscribe", "application/json", bytes.NewReader(payload))
		if err == nil && resp.StatusCode == http.StatusOK {
			log.Println("Successfully subscribed to the PaymentSucceeded event.")
			if closeErr := resp.Body.Close(); closeErr != nil {
				log.Printf("Error closing response body after successful subscription: %v", closeErr)
			}
			return
		}

		if err != nil {
			log.Printf("Failed subscription attempt: %v. Retry in %v...", err, retryDelay)
		} else {
			log.Printf("Attempt to subscribe returned status %d. I will try again in %v...", resp.StatusCode, retryDelay)
			if closeErr := resp.Body.Close(); closeErr != nil {
				log.Printf("Error closing response body after failed subscription: %v", closeErr)
			}
		}
		time.Sleep(retryDelay)
	}
	log.Fatalf("Failed to subscribe to the PaymentSucceeded event after %d attempts. Exit.", maxRetries)
}

// publishEvent sends an event to the Event Bus.
func publishEvent(eventType string, data interface{}) error {
	evt := Event{Type: eventType, Data: data}
	payload, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("error in the event payload marshal: %v", err)
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

// shipHandler handles the /ship request, processing the PaymentSucceeded events.
func shipHandler(w http.ResponseWriter, r *http.Request) {
	// Make sure that only POST requests are handled.
	if r.Method != http.MethodPost {
		http.Error(w, "Method not permitted", http.StatusMethodNotAllowed)
		return
	}

	var eventBusPayload EventBusPayload
	// Decodes the complete payload from the Event Bus.
	if err := json.NewDecoder(r.Body).Decode(&eventBusPayload); err != nil {
		http.Error(w, "Invalid payload from event bus", http.StatusBadRequest)
		log.Printf("Error decoding the payload from the Event Bus: %v", err)
		return
	}

	// Check the type of event.
	if eventBusPayload.Type != "PaymentSucceeded" {
		http.Error(w, "type of unexpected event", http.StatusBadRequest)
		log.Printf("Received unexpected event type: %s", eventBusPayload.Type)
		return
	}

	var req ShippingRequest
	// Decodes the 'Date' field (which is json.RawMessage) in the struct ShippingRequest.
	if err := json.Unmarshal(eventBusPayload.Data, &req); err != nil {
		http.Error(w, "Invalid PaymentSucceeded event data", http.StatusBadRequest)
		log.Printf("Error in the unmarshal of PaymentSucceeded Data in ShippingRequest: %v", err)
		return
	}

	log.Printf("Shipment processing for OrderID: %s (received from PaymentSucceeded event)", req.OrderID)

	eventType := "ShippingSucceeded"
	// Use the external shipping simulator.
	if err := shippingSimulator.ProcessShipping(req.OrderID); err != nil {
		eventType = "ShippingFailed"
		log.Printf("Shipment processing failed for OrderID %s: %v", req.OrderID, err)
	} else {
		log.Printf("Shipment successfully processed for OrderID: %s", req.OrderID)
	}

	// Publish the outcome event (ShippingSucceeded or ShippingFailed).
	if err := publishEvent(eventType, map[string]string{"OrderID": req.OrderID}); err != nil {
		log.Printf("Publication %s failed: %v", eventType, err)
		http.Error(w, "event bus error", http.StatusInternalServerError)
		return
	}

	// If the shipment failed, it logs to show that there are no other services subscribed to ShippingFailed.
	if eventType == "ShippingFailed" {
		log.Printf("Attention: Event %s for OrderID %s published. No other services are currently subscribed to this clearing event.", eventType, req.OrderID)
	}
	w.WriteHeader(http.StatusOK)
}

func main() {
	shippingSimulator = NewExternalShippingSimulator() // Initialise Shipping Simulator

	// Subscribe to the Event Bus at start-up, with retry logic.
	subscribeToPaymentSucceeded()

	http.HandleFunc("/ship", shipHandler)

	log.Println("Shipping Service (choreo) listening on port :8083")
	log.Fatal(http.ListenAndServe(":8083", nil))
}
