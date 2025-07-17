package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/StitchMl/saga-demo/choreographer_saga/shared"
	inventorydb "github.com/StitchMl/saga-demo/common/data_store"
	events "github.com/StitchMl/saga-demo/common/types"
)

var eventBus *shared.EventBus

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

func main() {
	port := os.Getenv("AUTH_SERVICE_PORT")
	if port == "" {
		log.Fatal("AUTH_SERVICE_PORT non è settata")
	}

	rabbitMQURL := os.Getenv("RABBITMQ_URL")
	if rabbitMQURL == "" {
		log.Fatal("RABBITMQ_URL non impostata")
	}
	var err error
	eventBus, err = shared.NewEventBus(rabbitMQURL)
	if err != nil {
		log.Fatalf("Unable to create EventBus: %v", err)
	}
	defer eventBus.Close()

	for event, handler := range map[events.EventType]func(interface{}){
		events.UserRegisteredEvent: handleUserRegisteredEvent,
		events.UserLoginEvent:      handleUserLoginEvent,
		events.ValidateEvent:       handleValidateEvent,
	} {
		if err := eventBus.Subscribe(event, handler); err != nil {
			log.Fatalf("Subscription error %s: %v", event, err)
		}
	}

	log.Printf("Auth Service (Choreo) started on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
