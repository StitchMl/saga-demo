package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// OrderCreated represents the creation event of an order.
type OrderCreated struct {
	OrderID string  `json:"OrderID"`
	Amount  float64 `json:"Amount"`
}

var (
	// Defines an HTTP client with a timeout for requests.
	httpClient = &http.Client{
		Timeout: 5 * time.Second, // 5-second timeout to avoid indefinite blocking.
	}
)

func main() {
	http.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {
		// 1. Explicit handling of the HTTP method.
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req OrderCreated
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid input", http.StatusBadRequest)
			log.Printf("Error decoding OrderCreated request: %v", err)
			return
		}

		// 2. Logging of the order received.
		log.Printf("Received request to create OrderID: %s with Amount: %.2f", req.OrderID, req.Amount)

		// Prepare the event payload to be published.
		eventPayload := map[string]interface{}{
			"Type": "OrderCreated",
			"Data": req,
		}

		// 3. Error handling of json.Marshal.
		payloadBytes, err := json.Marshal(eventPayload)
		if err != nil {
			http.Error(w, "internal server error: failed to marshal event payload", http.StatusInternalServerError)
			log.Printf("Error marshalling OrderCreated event payload: %v", err)
			return
		}

		// Publishes the OrderCreated event on the Event Bus.
		resp, err := httpClient.Post("http://event-bus:8070/publish", "application/json", bytes.NewReader(payloadBytes))
		if err != nil {
			// Includes the HTTP client error in the log.
			log.Printf("Failed to publish OrderCreated event to Event Bus: %v", err)
			http.Error(w, "event bus communication error", http.StatusInternalServerError)
			return
		}
		// Be sure to close the body of the answer and handle any closing errors.
		defer func() {
			if err := resp.Body.Close(); err != nil {
				log.Printf("Error closing response body from Event Bus: %v", err)
			}
		}()

		if resp.StatusCode != http.StatusOK {
			// Logs the non-OK status code received from the Event Bus.
			log.Printf("Event Bus returned non-OK status for OrderCreated: %d", resp.StatusCode)
			http.Error(w, "event bus rejected event", http.StatusInternalServerError)
			return
		}

		log.Printf("Successfully published OrderCreated event for OrderID: %s", req.OrderID)
		w.WriteHeader(http.StatusCreated)
	})

	log.Println("Order Service (choreo) listening on :8081")
	log.Fatal(http.ListenAndServe(":8081", nil))
}
