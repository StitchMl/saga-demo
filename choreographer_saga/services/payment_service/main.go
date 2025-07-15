package main

import (
	"encoding/json"
	"fmt"
	"github.com/StitchMl/saga-demo/choreographer_saga/shared"
	"github.com/StitchMl/saga-demo/common/events"
	"github.com/StitchMl/saga-demo/common/payment_gateway"
	"log"
	"os"
	"sync"
)

// In-memory database for payment transactions
var transactionsDB = struct {
	sync.RWMutex
	Data map[string]string // Map OrderID to transaction status (processed, reverted)
}{Data: make(map[string]string)}

var eventBus *shared.EventBus

func main() {
	rabbitMQURL := os.Getenv("RABBITMQ_URL") // Default URL of RabbitMQ
	if rabbitMQURL == "" {
		// Provide a safe fallback or fatal error if not set
		log.Fatal("RABBITMQ_URL environment variable not set.")
	}

	var err error
	eventBus, err = shared.NewEventBus(rabbitMQURL)
	if err != nil {
		log.Fatalf("Order Service: Failed to create event bus: %v", err)
	}
	defer eventBus.Close() // Make sure to close the connection when the service stops

	// Subscribe to events relevant to the payment service
	if err := eventBus.Subscribe(events.InventoryReservedEvent, handleInventoryReservedEvent); err != nil {
		log.Fatalf("Payment Service: Failed to subscribe to InventoryReservedEvent: %v", err)
	}
	if err := eventBus.Subscribe(events.InventoryReservationFailedEvent, handleInventoryReservationFailedEvent); err != nil {
		log.Fatalf("Payment Service: Failed to subscribe to InventoryReservationFailedEvent: %v", err)
	}
	if err := eventBus.Subscribe(events.RevertPaymentEvent, handleRevertPaymentEvent); err != nil {
		log.Fatalf("Payment Service: Failed to subscribe to RevertPaymentEvent: %v", err)
	}

	log.Println("Payment Service started, listening for events...")
	select {} // Keeps the service running indefinitely
}

// handleInventoryReservedEvent handles the reserved inventory event
func handleInventoryReservedEvent(eventPayload interface{}) {
	payloadBytes, err := json.Marshal(eventPayload)
	if err != nil {
		log.Printf("Payment Service: Error marshalling eventPayload for InventoryReservedEvent: %v", err)
		return
	}

	var payload events.InventoryReservedPayload
	err = json.Unmarshal(payloadBytes, &payload)
	if err != nil {
		log.Printf("Payment Service: Error unmarshalling payload [InventoryReservedPayload]: %v", err)
		return
	}

	log.Printf("Payment Service: Received InventoryReservedEvent for Order %s. Proceeding with payment.", payload.OrderID)

	// Simulates the logic of calculating the amount
	amount := 0.0
	for _, item := range payload.Items {
		amount += float64(item.Quantity) * item.Price
	} // Example: 10 per unit of a product
	customerID := "customer-" + payload.OrderID // Fictitious customer ID

	// Interact with the simulated payment gateway
	gatewayStatus, gatewayErr := payment_gateway.ProcessPayment(payload.OrderID, customerID, amount)

	transactionsDB.Lock()
	defer transactionsDB.Unlock()

	if gatewayErr != nil || gatewayStatus != "success" {
		log.Printf("Payment Service: Payment for Order %s failed at Gateway. Error: %v, Gateway Status: %s", payload.OrderID, gatewayErr, gatewayStatus)
		transactionsDB.Data[payload.OrderID] = "failed"
		// Publish the payment failure event
		failPayload := events.PaymentFailedPayload{
			OrderID: payload.OrderID,
			Amount:  amount,
			Reason:  fmt.Sprintf("Payment gateway failed: %v", gatewayErr),
		}
		if err := eventBus.Publish(events.NewGenericEvent(events.PaymentFailedEvent, payload.OrderID, "Payment failed", failPayload)); err != nil {
			log.Printf("Payment Service: Failed to publish PaymentFailedEvent: %v", err)
		}

		// Send an event to compensate inventory (rollback)
		revertInventoryPayload := events.RevertInventoryPayload{
			OrderID: payload.OrderID,
			Items:   payload.Items,
			Reason:  "Payment failed, revert inventory",
		}
		if err := eventBus.Publish(events.NewGenericEvent(events.RevertInventoryEvent, payload.OrderID, "Revert inventory due to payment failure", revertInventoryPayload)); err != nil {
			log.Printf("Payment Service: Failed to publish RevertInventoryEvent: %v", err)
		}
		return
	}

	transactionsDB.Data[payload.OrderID] = "processed"
	log.Printf("Payment Service: Payment for Order %s processed by Gateway. Amount: %.2f", payload.OrderID, amount)

	// Publish a successfully processed payment event
	successPayload := events.PaymentProcessedPayload{
		OrderID: payload.OrderID,
		Amount:  amount,
	}
	if err := eventBus.Publish(events.NewGenericEvent(events.PaymentProcessedEvent, payload.OrderID, "Payment processed successfully", successPayload)); err != nil {
		log.Printf("Payment Service: Failed to publish PaymentProcessedEvent: %v", err)
	}
}

// handleInventoryReservationFailedEvent handles inventory reservation failure
// This service shouldn't process the payment if the inventory has not been reserved.
// Serves for logging or future state management logics.
func handleInventoryReservationFailedEvent(eventPayload interface{}) {
	payloadBytes, err := json.Marshal(eventPayload)
	if err != nil {
		log.Printf("Payment Service: Error marshalling eventPayload to bytes: %v", err)
		return
	}

	var payload events.InventoryReservedPayload
	err = json.Unmarshal(payloadBytes, &payload)
	if err != nil {
		log.Printf("Payment Service: Error unmarshalling payload bytes to InventoryReservedPayload: %v", err)
		return
	}
	log.Printf("Payment Service: Received InventoryReservationFailedEvent for Order %s. Payment will not proceed. Reason: %s", payload.OrderID, payload.Reason)
	// Here there is no action to be taken on the payment, but it is used for co-ordination (for example, notifying the user).
	// Could publish OrderRejectedEvent if the order service has not already done so.
	rejectPayload := events.OrderRejectedPayload{
		OrderID: payload.OrderID,
		Reason:  "Inventory reservation failed before payment attempt",
	}
	if err := eventBus.Publish(events.NewGenericEvent(events.OrderRejectedEvent, payload.OrderID, "Order rejected due to inventory failure", rejectPayload)); err != nil {
		log.Printf("Payment Service: Failed to publish OrderRejectedEvent: %v", err)
	}
}

// handleRevertPaymentEvent handles the payment offset request
func handleRevertPaymentEvent(eventPayload interface{}) {
	payloadBytes, err := json.Marshal(eventPayload)
	if err != nil {
		log.Printf("Payment Service: Error marshalling eventPayload for InventoryReservedEvent: %v", err)
		return
	}

	var payload events.InventoryReservedPayload
	err = json.Unmarshal(payloadBytes, &payload)
	if err != nil {
		log.Printf("Payment Service: Error unmarshalling payload bytes to InventoryReservedPayload: %v", err)
		return
	}

	log.Printf("Payment Service: Received RevertPaymentEvent for Order %s (Reason: %s)", payload.OrderID, payload.Reason)

	transactionsDB.Lock()
	defer transactionsDB.Unlock()

	currentLocalStatus := transactionsDB.Data[payload.OrderID]
	if currentLocalStatus != "processed" {
		log.Printf("Payment Service: Cannot revert payment for Order %s. Local status is %s, not 'processed'.", payload.OrderID, currentLocalStatus)
		// Don't publish a clearing failure event here, the caller will have to handle it.
		return
	}

	gatewayStatus, gatewayErr := payment_gateway.RevertPayment(payload.OrderID, payload.Reason)
	if gatewayErr != nil || gatewayStatus != "success" {
		log.Printf("Payment Service: Payment reversal for Order %s failed at Gateway. Error: %v, Gateway Status: %s", payload.OrderID, gatewayErr, gatewayStatus)
		// Publish a specific event for payment compensation failure if necessary
		return
	}

	transactionsDB.Data[payload.OrderID] = "reverted"
	log.Printf("Payment Service: Payment for Order %s successfully reverted by Gateway.", payload.OrderID)
}
