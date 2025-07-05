package main

import (
	"log"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

const (
	ordersPath   = "/orders/"
	cancelSuffix = "/cancel"
)

var store = make(map[string]bool)

func createOrder(w http.ResponseWriter, _ *http.Request) {
	id := uuid.New().String()
	store[id] = true
	w.Header().Set("Location", ordersPath+id)
	w.WriteHeader(http.StatusCreated)
}

func cancelOrder(w http.ResponseWriter, r *http.Request) {
	// Get the ID by removing prefix and suffix with strings functions.
	path := r.URL.Path
	id := strings.TrimSuffix(strings.TrimPrefix(path, ordersPath), cancelSuffix)

	if store[id] {
		delete(store, id)
		w.WriteHeader(http.StatusOK)
	} else {
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func main() {
	http.HandleFunc("/orders", createOrder)
	http.HandleFunc(ordersPath, cancelOrder)
	log.Println("Order Service listening on :8081")
	log.Fatal(http.ListenAndServe(":8081", nil))
}
