package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	inventorydb "github.com/StitchMl/saga-demo/common/data_store"
	events "github.com/StitchMl/saga-demo/common/types"
	"github.com/google/uuid"
)

const (
	errorInvalidInput     = "invalid input"
	errorMethodNotAllowed = "method not allowed"
	ContentTypeJSON       = "application/json"
	ContentType           = "Content-Type"
)

func init() {
	inventorydb.InitDB()
}

/* ---------- HTTP handlers  ---------- */

// registerHandler handles user registration requests.
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
	// The password from the client is in the Password field
	hash, err := events.HashPassword(u.Password)
	if err != nil {
		http.Error(w, "failed to hash password", http.StatusInternalServerError)
		return
	}
	u.PasswordHash = hash
	u.Password = "" // Clear plain text password

	if u.ID == "" {
		u.ID = uuid.NewString()
	}

	inventorydb.DB.Users.Lock()
	defer inventorydb.DB.Users.Unlock()
	for _, usr := range inventorydb.DB.Users.Data {
		if usr.Username == u.Username {
			http.Error(w, "user exists", http.StatusConflict)
			return
		}
	}
	inventorydb.DB.Users.Data = append(inventorydb.DB.Users.Data, u)

	w.Header().Set(ContentType, ContentTypeJSON)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"customer_id": u.ID,
	})
}

// loginHandler handles user login requests.
func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, errorMethodNotAllowed, http.StatusMethodNotAllowed)
		return
	}
	var req events.AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid input", http.StatusBadRequest)
		return
	}

	inventorydb.DB.Users.RLock()
	defer inventorydb.DB.Users.RUnlock()

	for _, user := range inventorydb.DB.Users.Data {
		if user.Username == req.Username &&
			events.CheckPassword(user.PasswordHash, req.Password) == nil {
			w.Header().Set(ContentType, ContentTypeJSON)
			_ = json.NewEncoder(w).Encode(map[string]string{"customer_id": user.ID})
			return
		}
	}
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

// validateHandler responds to POST request /validate
func validateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, errorMethodNotAllowed, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		CustomerID string `json:"customer_id"`
	}
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

	w.Header().Set(ContentType, ContentTypeJSON)
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

// healthHandler returns a simple health check response.
func healthHandler(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

func main() {
	port := os.Getenv("AUTH_SERVICE_PORT")
	if port == "" {
		log.Fatal("AUTH_SERVICE_PORT missing")
	}

	// REST API
	http.HandleFunc("/register", registerHandler)
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/validate", validateHandler)
	http.HandleFunc("/health", healthHandler)

	log.Printf("[Auth-C] listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
