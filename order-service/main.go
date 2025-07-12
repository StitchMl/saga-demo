package main

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"io"
	"log"
	"net/http"
	"sync"
)

const (
	ordersPath = "/orders" // Basic order path
)

// Order is a simple representation of an order for the JSON response.
type Order struct {
	OrderID string `json:"orderID"`
}

// store is the in-memory 'database' of orders.
// A sync.RWMutex protects it to guarantee thread-safety.
var (
	store = make(map[string]bool)
	mu    sync.RWMutex // Mutex to protect the map store
)

// createOrder handles the creation of a new order.
// Now generates a UUID and returns it in the body of the JSON response,
// allowing the orchestrator to use it for the next steps.
func createOrder(w http.ResponseWriter, r *http.Request) {
	// Check that the HTTP method is POST.
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := uuid.New().String()

	mu.Lock() // Get write lock to modify the map
	store[id] = true
	mu.Unlock() // Release the lock

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", ordersPath+"/"+id) // Returns the URI of the created order
	w.WriteHeader(http.StatusCreated)

	// Returns the ID generated in the body of the JSON response.
	if err := json.NewEncoder(w).Encode(Order{OrderID: id}); err != nil {
		// Handles the JSON encoding error.
		log.Printf("Error encoding response for order %s: %v", id, err)
	}
}

// cancelOrder handles the cancellation of an order.
// Extracts the ID from the path using mux.Vars.
func cancelOrder(w http.ResponseWriter, r *http.Request) {
	// Check that the HTTP method is POST (consistent with the orchestrator, but DELETE would be more semantic).
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	vars := mux.Vars(r)
	id := vars["id"] // Extracts the ID from the path

	mu.Lock() // Get write lock to modify the map
	if store[id] {
		delete(store, id)
		mu.Unlock()                         // Release the lock before writing the answer
		w.WriteHeader(http.StatusNoContent) // 204 No Content is more correct for successful deletions
	} else {
		mu.Unlock() // Release the lock
		http.Error(w, "not found", http.StatusNotFound)
	}
}

// healthCheckHandler responds with 200 OK for healthchecks.
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	if _, err := io.WriteString(w, "OK"); err != nil {
		log.Printf("Warning: Error writing health check response: %v", err)
	}
}

func main() {
	router := mux.NewRouter()

	// Route for the creation of orders (POST /orders)
	router.HandleFunc(ordersPath, createOrder).Methods("POST")

	// Route for the cancellation of orders (POST /orders/{id}/cancel)
	// Uses a path variable 'id' to remove the UUID.
	router.HandleFunc(ordersPath+"/{id}/cancel", cancelOrder).Methods("POST")

	// Register the health check handler with the mux router
	router.HandleFunc("/health", healthCheckHandler).Methods("GET")

	log.Println("Order Service listening on :8081")
	log.Fatal(http.ListenAndServe(":8081", router)) // Use the router
}
