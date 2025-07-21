package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"

	"github.com/StitchMl/saga-demo/choreographer_saga/shared"
	"github.com/StitchMl/saga-demo/common/payment_gateway"
	events "github.com/StitchMl/saga-demo/common/types"
)

const payloadErr = "Payment Service: Payload error: %v"

// In-memory database for payment transactions
var (
	eventBus           *shared.EventBus
	paymentAmountLimit float64
	txDB               = struct {
		sync.RWMutex
		Data map[string]string
	}{Data: make(map[string]string)}
)

func main() {
	rabbitMQURL := os.Getenv("RABBITMQ_URL")
	if rabbitMQURL == "" {
		log.Fatal("RABBITMQ_URL non impostata.")
	}

	limitStr := os.Getenv("PAYMENT_AMOUNT_LIMIT")
	if limitStr == "" {
		log.Fatal("PAYMENT_AMOUNT_LIMIT not set")
	}
	var err error
	paymentAmountLimit, err = strconv.ParseFloat(limitStr, 64)
	if err != nil {
		log.Fatalf("Invalid PAYMENT_AMOUNT_LIMIT: %v", err)
	}

	eventBus, err = shared.NewEventBus(rabbitMQURL)
	if err != nil {
		log.Fatalf("Unable to create EventBus: %v", err)
	}
	defer eventBus.Close()

	subscribe(events.InventoryReservedEvent, handleInventoryReserved)
	subscribe(events.RevertInventoryEvent, handleRevertPayment)

	log.Println("Payment Service initiated.")
	select {}
}

// ---------- handlers ----------

// handleInventoryReserved handles the reserved inventory event and processes the payment.
func handleInventoryReserved(event events.GenericEvent) {
	var payload events.InventoryRequestPayload
	if err := mapP(event.Payload, &payload); err != nil {
		log.Printf(payloadErr, err)
		return
	}

	// Check payment limit
	if payload.Amount > paymentAmountLimit {
		reason := fmt.Sprintf("amount %.2f exceeds limit of %.2f", payload.Amount, paymentAmountLimit)
		publish(events.PaymentFailedEvent, payload.OrderID, "Payment failed", events.OrderStatusUpdatePayload{
			OrderID: payload.OrderID,
			Reason:  reason,
			Total:   payload.Amount,
		})
		return
	}

	txDB.RLock()
	status, exists := txDB.Data[payload.OrderID]
	txDB.RUnlock()
	if exists && status == "processed" {
		log.Printf("Payment for order %s already processed.", payload.OrderID)
		return
	}

	err := payment_gateway.ProcessPayment(payload.OrderID, payload.CustomerID, payload.Amount)

	txDB.Lock()
	defer txDB.Unlock()

	if err != nil {
		txDB.Data[payload.OrderID] = "failed"
		reason := err.Error()

		// Publish payment failure, other services will react to it.
		publish(events.PaymentFailedEvent, payload.OrderID, "Payment failed", events.OrderStatusUpdatePayload{
			OrderID: payload.OrderID,
			Reason:  reason,
			Total:   payload.Amount,
		})
		return
	}
	txDB.Data[payload.OrderID] = "processed"

	// Publish payment success, order service will react to it.
	publish(events.PaymentProcessedEvent, payload.OrderID, "Payment successful", events.PaymentPayload{
		OrderID:    payload.OrderID,
		CustomerID: payload.CustomerID,
		Amount:     payload.Amount,
	})
}

// handleRevertPayment handles the payment reversal request.
func handleRevertPayment(event events.GenericEvent) {
	var payload events.InventoryRequestPayload
	if err := mapP(event.Payload, &payload); err != nil {
		log.Printf(payloadErr, err)
		return
	}

	log.Printf("Reverting payment for order %s", payload.OrderID)
	err := payment_gateway.RevertPayment(payload.OrderID, payload.Reason)
	if err != nil {
		log.Printf("Failed to revert payment for order %s: %v", payload.OrderID, err)
		// In a real scenario, this might require manual intervention or a retry mechanism.
	}

	txDB.Lock()
	txDB.Data[payload.OrderID] = "reverted"
	txDB.Unlock()
}

// ---------- util ----------

// mapP simplifies the conversion of the eventPayload
func mapP(src interface{}, dst interface{}) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

// publish simplifies the publication of events
func publish(t events.EventType, id, msg string, pl events.EventPayload) {
	if err := eventBus.Publish(events.NewGenericEvent(t, id, msg, pl)); err != nil {
		log.Printf("publish %s: %v", t, err)
	}
}

// subscribe simplifies the subscription to events
func subscribe(t events.EventType, h shared.EventHandler) {
	if err := eventBus.Subscribe(t, h); err != nil {
		log.Fatalf("subscribe %s: %v", t, err)
	}
}
