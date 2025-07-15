package payment_gateway

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// simulatedGatewayDB simulates the internal database of a payment gateway.
// Map OrderID to the status of the transaction in the gateway (for example, 'completed', 'refunded', 'pending', 'failed').
var simulatedGatewayDB = struct {
	sync.RWMutex
	Transactions map[string]string
}{Transactions: make(map[string]string)}

func init() {
	log.Println("Simulated Payment Gateway initialized.")
}

// ProcessPayment simulates the processing of a payment by the gateway.
// Returns 'success' or 'failure' and an error if something goes wrong in the simulator.
func ProcessPayment(orderID, customerID string, amount float64) (string, error) {
	log.Printf("[Simulated Gateway] Processing payment for Order %s (Customer: %s, Amount: %.2f)", orderID, customerID, amount)

	// Simulates a processing delay
	time.Sleep(100 * time.Millisecond)

	simulatedGatewayDB.Lock()
	defer simulatedGatewayDB.Unlock()

	if amount > 100.00 {
		log.Printf("[Simulated Gateway] Payment for Order %s FAILED (simulated: amount %.2f exceeds limit).", orderID, amount)
		simulatedGatewayDB.Transactions[orderID] = "failed"
		return "failure", fmt.Errorf("simulated payment failure: amount %.2f exceeds allowed limit", amount)
	}

	// In a real gateway, there would be validation checks,
	// interactions with banks, and so on Here we only simulate success.
	simulatedGatewayDB.Transactions[orderID] = "completed"
	log.Printf("[Simulated Gateway] Payment for Order %s completed.", orderID)

	return "success", nil
}

// RevertPayment simulates the cancellation/repayment of a payment by the gateway.
// Returns 'success' or 'failure' and an error.
func RevertPayment(orderID, reason string) (string, error) {
	log.Printf("[Simulated Gateway] Attempting to revert payment for Order %s (Reason: %s)", orderID, reason)

	// Simulates a processing delay
	time.Sleep(50 * time.Millisecond)

	simulatedGatewayDB.Lock()
	defer simulatedGatewayDB.Unlock()

	currentStatus, exists := simulatedGatewayDB.Transactions[orderID]
	if !exists || currentStatus != "completed" {
		log.Printf("[Simulated Gateway] Cannot revert payment for Order %s. Current status: %s (Exists: %t)", orderID, currentStatus, exists)
		return "failure", fmt.Errorf("payment for order %s is not in a 'completed' state or does not exist", orderID)
	}

	simulatedGatewayDB.Transactions[orderID] = "refunded"
	log.Printf("[Simulated Gateway] Payment for Order %s reverted.", orderID)

	return "success", nil
}
