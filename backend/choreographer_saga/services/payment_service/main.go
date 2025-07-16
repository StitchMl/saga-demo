package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/StitchMl/saga-demo/choreographer_saga/shared"
	"github.com/StitchMl/saga-demo/common/payment_gateway"
	"github.com/StitchMl/saga-demo/common/types"
)

const payloadErrorLogFmt = "Payment Service: Payload error: %v"

// In-memory database for payment transactions
var transactionsDB = struct {
	sync.RWMutex
	Data map[string]string // Map OrderID to transaction status (processed, reverted)
}{Data: make(map[string]string)}

var eventBus *shared.EventBus

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

	handlers := map[events.EventType]func(interface{}){
		events.InventoryReservedEvent:          handleInventoryReservedEvent,
		events.InventoryReservationFailedEvent: handleInventoryReservationFailedEvent,
		events.RevertPaymentEvent:              handleRevertPaymentEvent,
	}
	for event, handler := range handlers {
		if err := eventBus.Subscribe(event, handler); err != nil {
			log.Fatalf("Error subscribing to %s: %v", event, err)
		}
	}

	log.Println("Payment Service initiated.")
	select {}
}

// handleInventoryReservedEvent handles the reserved inventory event
func handleInventoryReservedEvent(eventPayload interface{}) {
	var payload events.InventoryReservedPayload
	if err := mapPayload(eventPayload, &payload); err != nil {
		log.Printf(payloadErrorLogFmt, err)
		return
	}

	amount := 0.0
	for _, item := range payload.Items {
		amount += float64(item.Quantity) * item.Price
	}
	customerID := "customer-" + payload.OrderID

	status, err := payment_gateway.ProcessPayment(payload.OrderID, customerID, amount)

	transactionsDB.Lock()
	defer transactionsDB.Unlock()

	if err != nil || status != "success" {
		transactionsDB.Data[payload.OrderID] = "failed"
		publishEvent(events.PaymentFailedEvent, payload.OrderID, "Payment failed", events.PaymentPayload{
			OrderID: payload.OrderID, Amount: amount, Reason: fmt.Sprintf("Gateway error: %v", err),
		})
		publishEvent(events.RevertInventoryEvent, payload.OrderID, "Revert inventory", events.InventoryRequestPayload{
			OrderID: payload.OrderID, Items: payload.Items, Reason: "Payment failed, revert inventory",
		})
		return
	}

	transactionsDB.Data[payload.OrderID] = "processed"
	publishEvent(events.PaymentProcessedEvent, payload.OrderID, "Payment processed", events.PaymentPayload{
		OrderID: payload.OrderID, Amount: amount,
	})
}

// mapPayload simplifies the conversion of the eventPayload
func mapPayload(src interface{}, dst interface{}) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

// publishEvent simplifies the publication of events
func publishEvent(eventType events.EventType, orderID, msg string, payload interface{}) {
	if err := eventBus.Publish(events.NewGenericEvent(eventType, orderID, msg, payload)); err != nil {
		log.Printf("Payment Service: Error publishing event %s: %v", eventType, err)
	}
}

// handleInventoryReservationFailedEvent handles inventory reservation failure
func handleInventoryReservationFailedEvent(eventPayload interface{}) {
	var payload events.InventoryReservedPayload
	if err := mapPayload(eventPayload, &payload); err != nil {
		log.Printf(payloadErrorLogFmt, err)
		return
	}
	log.Printf("Payment Service: InventoryReservationFailedEvent for Order %s. Reason: %s", payload.OrderID, payload.Reason)
	rejectPayload := events.OrderStatusUpdatePayload{
		OrderID: payload.OrderID,
		Reason:  "Inventory reservation failed before payment attempt",
		Success: false,
	}
	if err := eventBus.Publish(events.NewGenericEvent(events.OrderRejectedEvent, payload.OrderID, "Order rejected due to inventory failure", rejectPayload)); err != nil {
		log.Printf("Payment Service: Failed to publish OrderRejectedEvent: %v", err)
	}
}

// handleRevertPaymentEvent handles the payment offset request
func handleRevertPaymentEvent(eventPayload interface{}) {
	var payload events.InventoryReservedPayload
	if err := mapPayload(eventPayload, &payload); err != nil {
		log.Printf(payloadErrorLogFmt, err)
		return
	}

	log.Printf("Payment Service: Received RevertPaymentEvent for Order %s (Reason: %s)", payload.OrderID, payload.Reason)

	transactionsDB.Lock()
	defer transactionsDB.Unlock()

	if transactionsDB.Data[payload.OrderID] != "processed" {
		log.Printf("Payment Service: Cannot revert payment for Order %s. Status is not 'processed'.", payload.OrderID)
		return
	}

	if status, err := payment_gateway.RevertPayment(payload.OrderID, payload.Reason); err != nil || status != "success" {
		log.Printf("Payment Service: Payment reversal for Order %s failed. Error: %v, Status: %s", payload.OrderID, err, status)
		return
	}

	transactionsDB.Data[payload.OrderID] = "reverted"
	log.Printf("Payment Service: Payment for Order %s successfully reverted.", payload.OrderID)
}
