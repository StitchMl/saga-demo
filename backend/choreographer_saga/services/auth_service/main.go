package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/StitchMl/saga-demo/common/types"
)

func main() {
	port := os.Getenv("AUTH_SERVICE_PORT")
	if port == "" {
		log.Fatal("AUTH_SERVICE_PORT non è settata")
	}
	log.Printf("Auth Service (Mock) started on port %s", port)
	http.HandleFunc("/validate", validateHandler)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// validateHandler simulates credential/token validation logic.
func validateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req events.AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	resp := events.AuthResponse{CustomerID: req.CustomerID, Valid: true}
	status := http.StatusOK

	switch {
	case req.CustomerID == "":
		resp.Valid = false
		resp.Message = "Customer ID cannot be empty."
		status = http.StatusBadRequest
	case req.CustomerID == "unauthorized-user":
		resp.Valid = false
		resp.Message = "User is explicitly unauthorized."
		status = http.StatusUnauthorized
	default:
		time.Sleep(50 * time.Millisecond)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Auth Service: Error encoding response: %v", err)
	}
	log.Printf("Auth Service: Validated CustomerID: %s, Valid: %t", resp.CustomerID, resp.Valid)
}
