package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/StitchMl/saga-demo/choreographer_saga/shared"
	"github.com/StitchMl/saga-demo/common/payment_gateway"
	events "github.com/StitchMl/saga-demo/common/types"
)

const payloadErr = "Payment Service: Payload error: %v"

// In-memory database for payment transactions
var (
	eventBus *shared.EventBus
	txDB     = struct {
		sync.RWMutex
		Data map[string]string
	}{Data: make(map[string]string)}
)

func main() {
	rabbitMQURL := os.Getenv("RABBITMQ_URL")
	if rabbitMQURL == "" {
		log.Fatal("RABBITMQ_URL non impostata.")
	}

	var err error
	eventBus, err = shared.NewEventBus(rabbitMQURL)
	if err != nil {
		log.Fatalf("Unable to create EventBus: %v", err)
	}
	defer eventBus.Close()

	subscribe(events.InventoryReservedEvent, handleReserved)
	subscribe(events.InventoryReservationFailedEvent, handleInvFail)

	log.Println("Payment Service initiated.")
	select {}
}

// ---------- handlers ----------

// handleReserved handles the reserved inventory event
func handleReserved(p interface{}) {
	var pl events.InventoryReservedPayload
	if err := mapP(p, &pl); err != nil {
		log.Printf(payloadErr, err)
		return
	}

	amount := 0.0
	for _, it := range pl.Items {
		amount += float64(it.Quantity) * it.Price
	}
	status, err := payment_gateway.ProcessPayment(pl.OrderID, pl.CustomerID, amount)

	txDB.Lock()
	defer txDB.Unlock()

	if err != nil || status != "success" {
		txDB.Data[pl.OrderID] = "failed"

		publish(events.OrderRejectedEvent, pl.OrderID, "Payment failed", events.OrderStatusUpdatePayload{
			OrderID: pl.OrderID, Reason: fmt.Sprintf("gateway: %v", err), Success: false,
		})

		publish(events.RevertInventoryEvent, pl.OrderID, "Revert inventory", events.InventoryRequestPayload{
			OrderID: pl.OrderID, Items: pl.Items, Reason: "payment failed",
		})
		return
	}
	txDB.Data[pl.OrderID] = "processed"

	publish(events.OrderApprovedEvent, pl.OrderID, "Payment ok", events.OrderStatusUpdatePayload{
		OrderID: pl.OrderID, Success: true,
	})
}

// handleInvFail handles inventory reservation failure
func handleInvFail(p interface{}) {
	var pl events.InventoryReservationFailedPayload
	if err := mapP(p, &pl); err != nil {
		log.Printf(payloadErr, err)
		return
	}
	publish(events.OrderRejectedEvent, pl.OrderID, "Inventory reservation failed", events.OrderStatusUpdatePayload{
		OrderID: pl.OrderID, Reason: pl.Reason, Success: false,
	})
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
func publish(t events.EventType, id, msg string, pl interface{}) {
	if err := eventBus.Publish(events.NewGenericEvent(t, id, msg, pl)); err != nil {
		log.Printf("publish %s: %v", t, err)
	}
}

// subscribe simplifies the subscription to events
func subscribe(t events.EventType, h func(interface{})) {
	if err := eventBus.Subscribe(t, h); err != nil {
		log.Fatalf("subscribe %s: %v", t, err)
	}
}
