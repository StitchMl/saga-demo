package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// AuthRequest simulates the payload that the gateway would send to the Auth Service.
type AuthRequest struct {
	CustomerID string `json:"customer_id"`
}

// AuthResponse simulates the response that the Auth Service would give to the gateway.
type AuthResponse struct {
	CustomerID string `json:"customer_id"`
	Valid      bool   `json:"valid"`
	Message    string `json:"message,omitempty"`
}

func main() {
	log.Println("Auth Service (Mock) started on port 8090")
	http.HandleFunc("/validate", validateHandler)
	log.Fatal(http.ListenAndServe(":8090", nil))
}

// validateHandler simulates credential/token validation logic.
// For simplicity, only validates the customerID.
func validateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var resp AuthResponse
	resp.CustomerID = req.CustomerID
	resp.Valid = true // By default, consider

	// Simulates a specific user failing authentication (for example, to test error scenarios)
	if req.CustomerID == "unauthorized-user" {
		resp.Valid = false
		resp.Message = "User is explicitly unauthorized."
		w.WriteHeader(http.StatusUnauthorized)
	} else if req.CustomerID == "" {
		resp.Valid = false
		resp.Message = "Customer ID cannot be empty."
		w.WriteHeader(http.StatusBadRequest)
	} else {
		// Simulates a small network delay
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Auth Service: Error encoding response: %v", err)
	}
	log.Printf("Auth Service: Validated CustomerID: %s, Valid: %t", resp.CustomerID, resp.Valid)
}
