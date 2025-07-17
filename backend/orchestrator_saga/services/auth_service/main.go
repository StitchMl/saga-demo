package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	inventorydb "github.com/StitchMl/saga-demo/common/data_store"
	events "github.com/StitchMl/saga-demo/common/types"
)

func registerHandler(w http.ResponseWriter, r *http.Request) {
	var u events.User
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}
	inventorydb.DB.Users.Lock()
	defer inventorydb.DB.Users.Unlock()
	for _, user := range inventorydb.DB.Users.Data {
		if user.Username == u.Username {
			http.Error(w, "User exists", http.StatusConflict)
			return
		}
	}
	inventorydb.DB.Users.Data = append(inventorydb.DB.Users.Data, u)
	w.WriteHeader(http.StatusCreated)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	var u events.User
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}
	inventorydb.DB.Users.Lock()
	defer inventorydb.DB.Users.Unlock()
	var authenticated bool
	for _, user := range inventorydb.DB.Users.Data {
		if user.Username == u.Username {
			if err := events.CheckPassword(user.PasswordHash, u.PasswordHash); err == nil {
				authenticated = true
				break
			}
		}
	}
	if !authenticated {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	w.WriteHeader(http.StatusOK)
}

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
	resp := events.AuthResponse{CustomerID: req.CustomerID, Valid: false}
	status := http.StatusOK
	inventorydb.DB.Users.Lock()
	for _, user := range inventorydb.DB.Users.Data {
		if user.Username == req.CustomerID {
			resp.Valid = true
			break
		}
	}
	inventorydb.DB.Users.Unlock()
	if req.CustomerID == "" {
		resp.Valid = false
		resp.Message = "Customer ID cannot be empty."
		status = http.StatusBadRequest
	} else if req.CustomerID == "unauthorized-user" {
		resp.Valid = false
		resp.Message = "User is explicitly unauthorized."
		status = http.StatusUnauthorized
	} else if !resp.Valid {
		resp.Message = "User not found."
		status = http.StatusUnauthorized
	} else {
		time.Sleep(50 * time.Millisecond)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}

func main() {
	http.HandleFunc("/register", registerHandler)
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/validate", validateHandler)
	port := os.Getenv("AUTH_SERVICE_PORT")
	if port == "" {
		log.Fatal("AUTH_SERVICE_PORT non è settata")
	}
	log.Printf("Auth Service (Orchestrator) started on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
