package main

import (
	"encoding/json"
	inventorydb "github.com/StitchMl/saga-demo/common/data_store"
	events "github.com/StitchMl/saga-demo/common/types"
	"github.com/google/uuid"
	"log"
	"net/http"
	"os"
)

const (
	errorInvalidInput     = "invalid input"
	errorMethodNotAllowed = "method not allowed"
)

// ---------------- REGISTER -----------------
func registerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, errorMethodNotAllowed, http.StatusMethodNotAllowed)
		return
	}
	var u events.User
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		http.Error(w, errorInvalidInput, http.StatusBadRequest)
		return
	}
	hash, _ := events.HashPassword(u.PasswordHash)
	u.PasswordHash = hash

	if u.ID == "" {
		u.ID = uuid.NewString()
	}

	inventorydb.DB.Users.Lock()
	for _, usr := range inventorydb.DB.Users.Data {
		if usr.Username == u.Username {
			inventorydb.DB.Users.Unlock()
			http.Error(w, "user exists", http.StatusConflict)
			return
		}
	}
	inventorydb.DB.Users.Data = append(inventorydb.DB.Users.Data, u)
	inventorydb.DB.Users.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"customer_id": u.ID,
	})
}

// ---------------- LOGIN --------------------
func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, errorMethodNotAllowed, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, errorInvalidInput, http.StatusBadRequest)
		return
	}

	inventorydb.DB.Users.RLock()
	defer inventorydb.DB.Users.RUnlock()
	for _, user := range inventorydb.DB.Users.Data {
		if user.Username == req.Username &&
			events.CheckPassword(user.PasswordHash, req.Password) == nil {
			_ = json.NewEncoder(w).Encode(map[string]string{
				"customer_id": user.ID,
			})
			return
		}
	}
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

// validateHandler risponde alla richiesta POST /validate
func validateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, errorMethodNotAllowed, http.StatusMethodNotAllowed)
		return
	}

	var req events.AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	inventorydb.DB.Users.RLock()
	defer inventorydb.DB.Users.RUnlock()

	valid := false
	for _, user := range inventorydb.DB.Users.Data {
		if user.ID == req.CustomerID {
			valid = true
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if valid {
		_ = json.NewEncoder(w).Encode(events.AuthResponse{
			CustomerID: req.CustomerID,
			Valid:      true,
		})
	} else {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(events.AuthResponse{
			CustomerID: req.CustomerID,
			Valid:      false,
		})
	}
}

func healthHandler(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

func main() {
	port := os.Getenv("AUTH_SERVICE_PORT")
	if port == "" {
		log.Fatal("AUTH_SERVICE_PORT non impostata")
	}

	http.HandleFunc("/register", registerHandler)
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/validate", validateHandler)
	http.HandleFunc("/health", healthHandler)

	log.Printf("[Auth‑O] listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
