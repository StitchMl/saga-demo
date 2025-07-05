package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

// Event represents the published payload
type Event struct {
	Type string          `json:"Type"`
	Data json.RawMessage `json:"Data"`
}

// SubscriberRequest defines the structure for subscription/unsubscription requests.
type SubscriberRequest struct {
	Type string `json:"Type"`
	Url  string `json:"Url"`
}

var (
	subscribers = make(map[string][]string)
	mu          sync.Mutex
	// Adds an http.Client with a timeout for publication calls.
	httpClient = &http.Client{
		Timeout: 5 * time.Second,
	}
)

// writeCORS adds the necessary CORS headers.
func writeCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, DELETE, OPTIONS") // Aggiunto DELETE
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

// dispatchToSubscriber handles the dispatch of an event to a single subscribed URL.
// This function is extracted from the goroutine in the publish function to reduce complexity.
func dispatchToSubscriber(url string, eventBody []byte, eventType string) {
	resp, err := httpClient.Post(url, "application/json", bytes.NewReader(eventBody))
	if err != nil {
		// For a real system, a retry/backoff logic could be implemented here.
		log.Printf("publish: POST %s failed for event type %s: %v", url, eventType, err)
		return
	}
	// Be sure to read and close the body of the answer to re-use the connection.
	// Handle errors from io.Copy and resp.Body.Close().
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		log.Printf("publish: error discarding response body from %s for event type %s: %v", url, eventType, err)
	}
	if err := resp.Body.Close(); err != nil {
		log.Printf("publish: error closing response body from %s for event type %s: %v", url, eventType, err)
	}
	if resp.StatusCode >= 300 {
		log.Printf("publish: non-OK %d from %s for event type %s", resp.StatusCode, url, eventType)
	}
}

// publish manages the publication of events.
func publish(w http.ResponseWriter, r *http.Request) {
	writeCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "cannot read body", http.StatusBadRequest)
		log.Printf("publish read error: %v", err)
		return
	}
	var e Event
	if err := json.Unmarshal(body, &e); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		log.Printf("publish decode error: %v", err)
		return
	}

	mu.Lock()
	// Creates a copy of the URL slice to avoid race condition during iteration.
	sinks := append([]string{}, subscribers[e.Type]...)
	mu.Unlock()

	// Send events asynchronously, delegating the scent logic to `dispatchToSubscriber`.
	// Note: For a production system, you would use a worker pool
	// to limit concurrency and better manage resources.
	for _, url := range sinks {
		go dispatchToSubscriber(url, body, e.Type) // Now call the helper function
	}

	w.WriteHeader(http.StatusOK)
}

// handleSubscribe handles the logic of adding a subscription.
func handleSubscribe(w http.ResponseWriter, req SubscriberRequest) {
	mu.Lock()
	defer mu.Unlock()

	subscribers[req.Type] = append(subscribers[req.Type], req.Url)
	log.Printf("Subscribed URL %s to event type %s", req.Url, req.Type)
	w.WriteHeader(http.StatusOK)
}

// handleUnsubscribe handles the logic of removing a subscription.
func handleUnsubscribe(w http.ResponseWriter, req SubscriberRequest) {
	mu.Lock()
	defer mu.Unlock()

	var updatedURLs []string
	found := false
	// Filters all occurrences of the URL to be removed.
	for _, u := range subscribers[req.Type] {
		if u == req.Url {
			found = true
			continue // Skip URL to remove
		}
		updatedURLs = append(updatedURLs, u)
	}

	if found {
		if len(updatedURLs) == 0 {
			delete(subscribers, req.Type) // Remove the key completely if there are no more URLs
		} else {
			subscribers[req.Type] = updatedURLs
		}
		log.Printf("Unsubscribed URL %s from event type %s", req.Url, req.Type)
		w.WriteHeader(http.StatusNoContent) // 204 No Content is more semantic for DELETE
	} else {
		http.Error(w, fmt.Sprintf("Subscription for type %s and URL %s not found", req.Type, req.Url), http.StatusNotFound)
		log.Printf("Unsubscribe failed: Subscription for type %s and URL %s not found", req.Type, req.Url)
	}
}

// subscribeHandler handles subscriptions (POST) and unsubscriptions (DELETE).
// Now acts as a dispatcher for helper functions.
func subscribeHandler(w http.ResponseWriter, r *http.Request) {
	writeCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	var req SubscriberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		log.Printf("subscribe/unsubscribe decode error: %v", err)
		return
	}

	switch r.Method {
	case http.MethodPost: // Subscribe
		handleSubscribe(w, req)
	case http.MethodDelete: // Unsubscribe
		handleUnsubscribe(w, req)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func main() {
	http.HandleFunc("/publish", publish)
	http.HandleFunc("/subscribe", subscribeHandler) // Single handler for POST and DELETE
	log.Println("Event Bus listening on :8070")
	log.Fatal(http.ListenAndServe(":8070", nil))
}
