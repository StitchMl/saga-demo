package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
)

type OrderCreated struct {
	OrderID string  `json:"OrderID"`
	Amount  float64 `json:"Amount"`
}

func main() {
	http.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {
		var req OrderCreated
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid input", http.StatusBadRequest)
			return
		}
		// Publish OrderCreated event
		payload, _ := json.Marshal(map[string]interface{}{
			"Type": "OrderCreated",
			"Data": req,
		})
		resp, err := http.Post("http://event-bus:8070/publish", "application/json", bytes.NewReader(payload))
		if err != nil || resp.StatusCode != http.StatusOK {
			log.Printf("failed to publish OrderCreated: %v", err)
			http.Error(w, "event bus error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})
	log.Println("Order Service (choreo) listening on :8081")
	log.Fatal(http.ListenAndServe(":8081", nil))
}
