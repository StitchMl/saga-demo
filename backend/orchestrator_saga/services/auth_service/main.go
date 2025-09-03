package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
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
			ID:           "user1",
			Name:         "Mario Rossi",
			Email:        "mario.rossi@example.com",
			Username:     "user1",
			PasswordHash: func() string { h, _ := events.HashPassword("pass1"); return h }(),
		},
		{
			ID:           "user2",
			Name:         "Luca Bianchi",
			Email:        "luca.bianchi@example.com",
			Username:     "user2",
			PasswordHash: func() string { h, _ := events.HashPassword("pass2"); return h }(),
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

	// Password hash and cleaning
	hash, _ := events.HashPassword(u.Password)
	u.PasswordHash = hash
	u.Password = ""

	// ALWAYS generate the stable ID with ns past.
	u.ID = events.StableCustomerID(u.Username, u.NS)

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
	var req events.AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, errorInvalidInput, http.StatusBadRequest)
		return
	}

	UsersDB.RLock()
	defer UsersDB.RUnlock()

	for _, user := range UsersDB.Data {
		if user.Username == req.Username &&
			events.CheckPassword(user.PasswordHash, req.Password) == nil {

			// Calculate deterministic SID with the gateway ns
			sid := events.StableCustomerID(user.Username, req.NS)

			// In-place migration of the record into the flow DB O
			UsersDB.RUnlock()
			UsersDB.Lock()
			for i := range UsersDB.Data {
				if UsersDB.Data[i].Username == user.Username {
					UsersDB.Data[i].ID = sid
					break
				}
			}
			UsersDB.Unlock()
			UsersDB.RLock()

			w.Header().Set(ContentType, ContentTypeJSON)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"customer_id": sid,
				"status":      "success",
				"ns":          req.NS,
			})
			return
		}
	}
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

func parseNS(ns string) (uuid.UUID, bool) {
	s := strings.TrimSpace(ns)
	if s == "" {
		return uuid.UUID{}, false
	}
	p, err := uuid.Parse(s)
	return p, err == nil
}

func validateInUsersDB(customerID string, parsedNS uuid.UUID, haveNS bool) (valid bool, toNormalizeUser, newSID string) {
	UsersDB.RLock()
	defer UsersDB.RUnlock()

	for _, user := range UsersDB.Data {
		// 1) match ID saved
		if user.ID == customerID {
			return true, "", ""
		}
		// 2) match ID calculated with ns for EXISTING USER
		if haveNS {
			uname := strings.ToLower(strings.TrimSpace(user.Username))
			sid := uuid.NewSHA1(parsedNS, []byte(uname)).String()
			if sid == customerID {
				if user.ID != sid {
					return true, user.Username, sid
				}
				return true, "", ""
			}
		}
	}
	return false, "", ""
}

func normalizeUserIDInUsersDB(username, sid string) {
	UsersDB.Lock()
	for i := range UsersDB.Data {
		if UsersDB.Data[i].Username == username {
			UsersDB.Data[i].ID = sid
			break
		}
	}
	UsersDB.Unlock()
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

	parsedNS, haveNS := parseNS(req.NS)
	valid, toNormalizeUser, newSID := validateInUsersDB(req.CustomerID, parsedNS, haveNS)

	if valid && toNormalizeUser != "" {
		normalizeUserIDInUsersDB(toNormalizeUser, newSID)
	}

	w.Header().Set(ContentType, ContentTypeJSON)
	if valid {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"customer_id": req.CustomerID,
			"valid":       true,
			"status":      "success",
		})
		return
	}

	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"customer_id": req.CustomerID,
		"valid":       false,
		"status":      "error",
		"message":     "Invalid customer ID",
	})
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
