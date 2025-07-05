package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
)

// ShippingRequest represents the incoming payload
type ShippingRequest struct {
	OrderID string `json:"OrderID"`
}

// mustPost performs a POST, checks for errors and closes Body safely.
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

func main() {
	// 1. Subscription a PaymentSucceeded su Event Bus
	subMsg := map[string]string{
		"Type": "PaymentSucceeded",
		"Url":  "http://shipping-service-choreo:8083/ship",
	}
	subPayload, err := json.Marshal(subMsg)
	if err != nil {
		log.Fatalf("failed to marshal subscription payload: %v", err)
	}
	mustPost("http://event-bus:8070/subscribe", subPayload, "Subscription")

	// 2. Handler per /ship
	http.HandleFunc("/ship", func(w http.ResponseWriter, r *http.Request) {
		var req ShippingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid input", http.StatusBadRequest)
			return
		}

		// 3. Shipping logic simulation
		eventType := "ShippingSucceeded"
		if req.OrderID == "fail" {
			eventType = "ShippingFailed"
		}

		// 4. Publish event with mustPost
		evt := map[string]interface{}{
			"Type": eventType,
			"Data": map[string]string{"OrderID": req.OrderID},
		}
		payload, _ := json.Marshal(evt)
		mustPost("http://event-bus:8070/publish", payload, eventType+" publish")

		// 5. Client response
		if eventType == "ShippingFailed" {
			http.Error(w, "shipping error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	log.Println("Shipping Service (choreo) listening on :8083")
	log.Fatal(http.ListenAndServe(":8083", nil))
}
