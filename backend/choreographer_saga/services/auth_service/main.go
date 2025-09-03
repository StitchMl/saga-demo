package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

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

	// Hash password and plain text cleaning
	hash, err := events.HashPassword(u.Password)
	if err != nil {
		http.Error(w, "failed to hash password", http.StatusInternalServerError)
		return
	}
	u.PasswordHash = hash
	u.Password = ""

	// ALWAYS generate the stable ID from the namespace passed
	u.ID = events.StableCustomerID(u.Username, u.NS)

	// Entry if username does not already exist in this DB (flow C)
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

			// Calculate deterministic SID from the gateway ns.
			sid := events.StableCustomerID(user.Username, req.NS)

			// In-place migration of the record into the flow C DB.
			inventorydb.DB.Users.RUnlock()
			inventorydb.DB.Users.Lock()
			for i := range inventorydb.DB.Users.Data {
				if inventorydb.DB.Users.Data[i].Username == user.Username {
					inventorydb.DB.Users.Data[i].ID = sid
					break
				}
			}
			inventorydb.DB.Users.Unlock()
			inventorydb.DB.Users.RLock()

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

func validateInDB(customerID string, parsedNS uuid.UUID, haveNS bool) (valid bool, toNormalizeUser, newSID string) {
	inventorydb.DB.Users.RLock()
	defer inventorydb.DB.Users.RUnlock()

	for _, user := range inventorydb.DB.Users.Data {
		// 1) direct match on saved ID
		if user.ID == customerID {
			return true, "", ""
		}
		// 2) match calculated with ns for EXISTING USER
		if haveNS {
			uname := strings.ToLower(strings.TrimSpace(user.Username))
			sid := uuid.NewSHA1(parsedNS, []byte(uname)).String()
			if sid == customerID {
				// pianifica normalizzazione se necessario
				if user.ID != sid {
					return true, user.Username, sid
				}
				return true, "", ""
			}
		}
	}
	return false, "", ""
}

func normalizeUserID(username, sid string) {
	inventorydb.DB.Users.Lock()
	for i := range inventorydb.DB.Users.Data {
		if inventorydb.DB.Users.Data[i].Username == username {
			inventorydb.DB.Users.Data[i].ID = sid
			break
		}
	}
	inventorydb.DB.Users.Unlock()
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
	valid, toNormalizeUser, newSID := validateInDB(req.CustomerID, parsedNS, haveNS)

	// Eventual normalization (only if we found the user via ns)
	if valid && toNormalizeUser != "" {
		normalizeUserID(toNormalizeUser, newSID)
	}

	w.Header().Set(ContentType, ContentTypeJSON)
	if valid {
		_ = json.NewEncoder(w).Encode(events.AuthResponse{
			CustomerID: req.CustomerID,
			Valid:      true,
		})
		return
	}

	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(events.AuthResponse{
		CustomerID: req.CustomerID,
		Valid:      false,
	})
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
