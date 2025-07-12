package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
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

// Common HTTP constants
const (
	ContentTypeHeader    = "Content-Type"
	ApplicationJSON      = "application/json"
	MethodNotAllowedMsg  = "Method not allowed"
	InvalidInputMsg      = "Invalid input"
	InternalServerErrMsg = "Internal server error"
)

var (
	subscribers = make(map[string][]string)
	mu          sync.Mutex
	httpClient  = &http.Client{
		Timeout: 5 * time.Second,
	}
	eventBusPort string // Configurable port
)

func init() {
	eventBusPort = getEnv("EVENT_BUS_PORT", "8070")
}

// Helper to get environment variables or use a default value
func getEnv(key, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return defaultValue
}

// structuredLog logs messages in a structured (JSON) format
func structuredLog(eventType string, fields map[string]interface{}) {
	logEntry := make(map[string]interface{})
	logEntry["timestamp"] = time.Now().Format(time.RFC3339)
	logEntry["service"] = "event-bus"
	logEntry["event_type"] = eventType
	for k, v := range fields {
		logEntry[k] = v
	}
	jsonLog, err := json.Marshal(logEntry)
	if err != nil {
		log.Printf("error marshalling log entry: %v, original: %v", err, logEntry)
		return
	}
	fmt.Println(string(jsonLog))
}

// writeCORS adds necessary CORS headers.
func writeCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

// dispatchToSubscriber handles sending an event to a single subscribed URL.
func dispatchToSubscriber(url string, eventBody []byte, eventType string) {
	resp, err := httpClient.Post(url, ApplicationJSON, bytes.NewReader(eventBody))
	if err != nil {
		log.Printf("publish: POST %s failed for event type %s: %v", url, eventType, err)
		return
	}
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
		http.Error(w, MethodNotAllowedMsg, http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, InvalidInputMsg, http.StatusBadRequest)
		log.Printf("publish read error: %v", err)
		return
	}
	var e Event
	if err := json.Unmarshal(body, &e); err != nil {
		http.Error(w, InvalidInputMsg, http.StatusBadRequest)
		log.Printf("publish decode error: %v", err)
		return
	}

	mu.Lock()
	sinks := append([]string{}, subscribers[e.Type]...)
	mu.Unlock()

	for _, url := range sinks {
		go dispatchToSubscriber(url, body, e.Type)
	}

	w.WriteHeader(http.StatusOK)
}

// handleSubscribe handles the logic of adding a subscription.
func handleSubscribe(w http.ResponseWriter, req SubscriberRequest) {
	mu.Lock()
	defer mu.Unlock()

	subscribers[req.Type] = append(subscribers[req.Type], req.Url)
	structuredLog("subscription_added", map[string]interface{}{"event_type": req.Type, "url": req.Url})
	w.WriteHeader(http.StatusOK)
}

// handleUnsubscribe handles the logic of removing a subscription.
func handleUnsubscribe(w http.ResponseWriter, req SubscriberRequest) {
	mu.Lock()
	defer mu.Unlock()

	var updatedURLs []string
	found := false
	for _, u := range subscribers[req.Type] {
		if u == req.Url {
			found = true
			continue
		}
		updatedURLs = append(updatedURLs, u)
	}

	if found {
		if len(updatedURLs) == 0 {
			delete(subscribers, req.Type)
		} else {
			subscribers[req.Type] = updatedURLs
		}
		structuredLog("subscription_removed", map[string]interface{}{"event_type": req.Type, "url": req.Url})
		w.WriteHeader(http.StatusNoContent)
	} else {
		http.Error(w, fmt.Sprintf("Subscription for type %s and URL %s not found", req.Type, req.Url), http.StatusNotFound)
		structuredLog("unsubscribe_failed", map[string]interface{}{"event_type": req.Type, "url": req.Url, "reason": "not found"})
	}
}

// subscribeHandler handles subscriptions (POST) and unsubscriptions (DELETE).
func subscribeHandler(w http.ResponseWriter, r *http.Request) {
	writeCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	var req SubscriberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, InvalidInputMsg, http.StatusBadRequest)
		log.Printf("subscribe/unsubscribe decode error: %v", err)
		return
	}

	switch r.Method {
	case http.MethodPost:
		handleSubscribe(w, req)
	case http.MethodDelete:
		handleUnsubscribe(w, req)
	default:
		http.Error(w, MethodNotAllowedMsg, http.StatusMethodNotAllowed)
	}
}

// healthCheckHandler responds with 200 OK for health checks.
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, MethodNotAllowedMsg, http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	if _, err := io.WriteString(w, "OK"); err != nil {
		log.Printf("Warning: Error writing health check response: %v", err)
	}
}

func main() {
	http.HandleFunc("/publish", publish)
	http.HandleFunc("/subscribe", subscribeHandler)
	http.HandleFunc("/health", healthCheckHandler) // New health check endpoint

	structuredLog("server_start", map[string]interface{}{"port": eventBusPort})
	if err := http.ListenAndServe(":"+eventBusPort, nil); err != nil {
		structuredLog("server_error", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
}
