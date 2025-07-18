package main

import (
	"encoding/json"
	"github.com/google/uuid"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/StitchMl/saga-demo/choreographer_saga/shared"
	inventorydb "github.com/StitchMl/saga-demo/common/data_store"
	events "github.com/StitchMl/saga-demo/common/types"
)

const (
	errorInvalidInput     = "invalid input"
	errorMethodNotAllowed = "method not allowed"
)

var eventBus *shared.EventBus

/* ---------- RabbitMQ subscription handlers ---------- */

// Handler for signing UserRegisteredEvent
func handleUserRegisteredEvent(data interface{}) {
	event, ok := data.(events.GenericEvent)
	if !ok {
		log.Printf("Invalid event type for UserRegisteredEvent")
		return
	}
	userMap, ok := event.Payload.(map[string]interface{})
	if !ok {
		log.Printf("Invalid payload for UserRegisteredEvent")
		return
	}
	var u events.User
	b, _ := json.Marshal(userMap)
	if err := json.Unmarshal(b, &u); err != nil {
		log.Printf("User unmarshalling error: %v", err)
		return
	}
	inventorydb.DB.Users.Lock()
	defer inventorydb.DB.Users.Unlock()
	for _, user := range inventorydb.DB.Users.Data {
		if user.Username == u.Username {
			log.Printf("User exists: %s", u.Username)
			return
		}
	}
	inventorydb.DB.Users.Data = append(inventorydb.DB.Users.Data, u)
	log.Printf("User registered: %s", u.Username)
}

// Handler for signing UserLoginEvent
func handleUserLoginEvent(data interface{}) {
	event, ok := data.(events.GenericEvent)
	if !ok {
		log.Printf("Invalid event type for UserLoginEvent")
		return
	}
	userMap, ok := event.Payload.(map[string]interface{})
	if !ok {
		log.Printf("Invalid payload for UserLoginEvent")
		return
	}
	var u events.User
	b, _ := json.Marshal(userMap)
	if err := json.Unmarshal(b, &u); err != nil {
		log.Printf("User unmarshalling error: %v", err)
		return
	}
	inventorydb.DB.Users.Lock()
	defer inventorydb.DB.Users.Unlock()
	found := false
	for _, user := range inventorydb.DB.Users.Data {
		if user.Username == u.Username {
			if err := events.CheckPassword(user.PasswordHash, u.PasswordHash); err == nil {
				found = true
				break
			}
		}
	}
	if !found {
		log.Printf("Login failed for user: %s", u.Username)
		return
	}
	log.Printf("User login: %s", u.Username)
}

// ValidateEvent subscription handler
func handleValidateEvent(data interface{}) {
	event, ok := data.(events.GenericEvent)
	if !ok {
		log.Printf("Invalid event type for ValidateEvent")
		return
	}
	reqMap, ok := event.Payload.(map[string]interface{})
	if !ok {
		log.Printf("Invalid payload for ValidateEvent")
		return
	}
	var req events.AuthRequest
	b, _ := json.Marshal(reqMap)
	if err := json.Unmarshal(b, &req); err != nil {
		log.Printf("AuthRequest unmarshalling error: %v", err)
		return
	}
	inventorydb.DB.Users.Lock()
	valid := false
	for _, user := range inventorydb.DB.Users.Data {
		if user.ID == req.CustomerID {
			valid = true
			break
		}
	}
	inventorydb.DB.Users.Unlock()
	resp := events.AuthResponse{CustomerID: req.CustomerID, Valid: valid}
	status := http.StatusOK
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
	log.Printf("Validate event: %+v, status: %d", resp, status)
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

	// pubblica evento su RabbitMQ
	ev := events.NewGenericEvent(
		events.UserRegisteredEvent, u.ID, "User registered", u,
	)
	_ = eventBus.Publish(ev)

	w.Header().Set("Content-Type", "application/json")
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
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid input", http.StatusBadRequest)
		return
	}
	inventorydb.DB.Users.RLock()
	defer inventorydb.DB.Users.RUnlock()
	for _, user := range inventorydb.DB.Users.Data {
		if user.Username == req.Username &&
			events.CheckPassword(user.PasswordHash, req.Password) == nil {
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

// healthHandler returns a simple health check response.
func healthHandler(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

func main() {
	port := os.Getenv("AUTH_SERVICE_PORT")
	if port == "" {
		log.Fatal("AUTH_SERVICE_PORT missing")
	}

	rmqURL := os.Getenv("RABBITMQ_URL")
	if rmqURL == "" {
		log.Fatal("RABBITMQ_URL missing")
	}
	var err error
	eventBus, err = shared.NewEventBus(rmqURL)
	if err != nil {
		log.Fatalf("EventBus: %v", err)
	}
	defer eventBus.Close()

	// subscriptions
	for ev, h := range map[events.EventType]func(interface{}){
		events.UserRegisteredEvent: handleUserRegisteredEvent,
		events.UserLoginEvent:      handleUserLoginEvent,
		events.ValidateEvent:       handleValidateEvent,
	} {
		if err := eventBus.Subscribe(ev, h); err != nil {
			log.Fatalf("subscribe %s: %v", ev, err)
		}
	}

	// REST API
	http.HandleFunc("/register", registerHandler)
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/validate", validateHandler)
	http.HandleFunc("/health", healthHandler)

	log.Printf("[Auth‑C] listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
