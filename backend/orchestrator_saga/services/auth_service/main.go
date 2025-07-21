package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"

	events "github.com/StitchMl/saga-demo/common/types"
	"github.com/google/uuid"
)

const (
	errorInvalidInput     = "invalid input"
	errorMethodNotAllowed = "method not allowed"
	ContentTypeJSON       = "application/json"
	ContentType           = "Content-Type"
)

// UsersDB is an in-memory database for the auth service.
var UsersDB = struct {
	sync.RWMutex
	Data []events.User
}{}

func initDB() {
	UsersDB.Lock()
	defer UsersDB.Unlock()
	UsersDB.Data = []events.User{
		{
			ID:           "user-1",
			Name:         "Mario Rossi",
			Email:        "mario.rossi@example.com",
			Username:     "mario.rossi",
			PasswordHash: func() string { h, _ := events.HashPassword("password123"); return h }(),
		},
		{
			ID:           "user-2",
			Name:         "Luca Bianchi",
			Email:        "luca.bianchi@example.com",
			Username:     "luca.bianchi",
			PasswordHash: func() string { h, _ := events.HashPassword("password456"); return h }(),
		},
	}
	log.Println("[AuthService] In-memory user database initialized.")
}

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
	hash, _ := events.HashPassword(u.Password)
	u.PasswordHash = hash
	u.Password = "" // Remove unencrypted password

	if u.ID == "" {
		u.ID = uuid.NewString()
	}

	UsersDB.Lock()
	for _, usr := range UsersDB.Data {
		if usr.Username == u.Username {
			UsersDB.Unlock()
			http.Error(w, "user exists", http.StatusConflict)
			return
		}
	}
	UsersDB.Data = append(UsersDB.Data, u)
	UsersDB.Unlock()

	w.Header().Set(ContentType, ContentTypeJSON)
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

	UsersDB.RLock()
	defer UsersDB.RUnlock()
	for _, user := range UsersDB.Data {
		if user.Username == req.Username &&
			events.CheckPassword(user.PasswordHash, req.Password) == nil {
			w.Header().Set(ContentType, ContentTypeJSON)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"customer_id": user.ID,
				"status":      "success",
			})
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

	var req events.AuthResponse
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	UsersDB.RLock()
	defer UsersDB.RUnlock()

	valid := false
	for _, user := range UsersDB.Data {
		if user.ID == req.CustomerID {
			valid = true
			break
		}
	}

	w.Header().Set(ContentType, ContentTypeJSON)
	if valid {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"customer_id": req.CustomerID,
			"valid":       true,
			"status":      "success",
		})
	} else {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"customer_id": req.CustomerID,
			"valid":       false,
			"status":      "error",
			"message":     "Invalid customer ID",
		})
	}
}

func healthHandler(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

func main() {
	port := os.Getenv("AUTH_SERVICE_PORT")
	if port == "" {
		log.Fatal("AUTH_SERVICE_PORT non impostata")
	}

	initDB()

	http.HandleFunc("/register", registerHandler)
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/validate", validateHandler)
	http.HandleFunc("/health", healthHandler)

	log.Printf("[Auth‑O] listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
