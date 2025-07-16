package payment_gateway

import (
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	inventorydb "github.com/StitchMl/saga-demo/backend/common/data_store"
	"github.com/StitchMl/saga-demo/backend/common/types"
)

// simulatedGatewayDB simulates the internal database of a payment gateway.
var simulatedGatewayDB = struct {
	sync.RWMutex
	Transactions map[string]events.Transaction
}{Transactions: make(map[string]events.Transaction)}

func init() {
	log.Println("Simulated Payment Gateway initialized.")
}

// ProcessPayment simulates the processing of a payment by the gateway.
func ProcessPayment(orderID, customerID string, amount float64) (string, error) {
	if customerID == "" {
		return "failure", fmt.Errorf("customerID non valido")
	}
	found := false
	inventorydb.DB.Orders.RLock()
	for _, order := range inventorydb.DB.Orders.Data {
		if order.CustomerID == customerID {
			found = true
			break
		}
	}
	inventorydb.DB.Orders.RUnlock()
	if !found {
		return "failure", fmt.Errorf("customerID non trovato in data_store")
	}

	simulatedGatewayDB.Lock()
	tx, exists := simulatedGatewayDB.Transactions[orderID]
	if exists && tx.Status == "completed" {
		simulatedGatewayDB.Unlock()
		return "success", nil
	}
	simulatedGatewayDB.Transactions[orderID] = events.Transaction{
		OrderID:    orderID,
		CustomerID: customerID,
		Amount:     amount,
		Status:     "pending",
		Timestamp:  time.Now(),
		Attempts:   tx.Attempts + 1,
	}
	simulatedGatewayDB.Unlock()

	time.Sleep(time.Duration(50+rand.Intn(150)) * time.Millisecond)

	simulatedGatewayDB.Lock()
	tx = simulatedGatewayDB.Transactions[orderID]
	tx.Status = "processing"
	tx.Timestamp = time.Now()
	simulatedGatewayDB.Transactions[orderID] = tx
	simulatedGatewayDB.Unlock()

	if rand.Float32() < 0.15 {
		simulatedGatewayDB.Lock()
		tx = simulatedGatewayDB.Transactions[orderID]
		tx.Status = "failed"
		tx.Timestamp = time.Now()
		tx.ErrorReason = "errore casuale simulato"
		simulatedGatewayDB.Transactions[orderID] = tx
		simulatedGatewayDB.Unlock()
		return "failure", fmt.Errorf("errore casuale simulato")
	}

	if amount > 100.00 {
		simulatedGatewayDB.Lock()
		tx = simulatedGatewayDB.Transactions[orderID]
		tx.Status = "failed"
		tx.Timestamp = time.Now()
		tx.ErrorReason = fmt.Sprintf("amount %.2f exceeds allowed limit", amount)
		simulatedGatewayDB.Transactions[orderID] = tx
		simulatedGatewayDB.Unlock()
		return "failure", fmt.Errorf("simulated payment failure: amount %.2f exceeds allowed limit", amount)
	}

	simulatedGatewayDB.Lock()
	tx = simulatedGatewayDB.Transactions[orderID]
	tx.Status = "completed"
	tx.Timestamp = time.Now()
	tx.ErrorReason = ""
	simulatedGatewayDB.Transactions[orderID] = tx
	simulatedGatewayDB.Unlock()
	return "success", nil
}

// RevertPayment simulates the cancellation/repayment of a payment by the gateway.
func RevertPayment(orderID, reason string) (string, error) {
	log.Printf("[Simulated Gateway] Reverting payment for Order %s (Reason: %s)", orderID, reason)
	time.Sleep(time.Duration(30+rand.Intn(70)) * time.Millisecond)

	simulatedGatewayDB.Lock()
	defer simulatedGatewayDB.Unlock()

	tx, exists := simulatedGatewayDB.Transactions[orderID]
	if !exists || tx.Status != "completed" {
		log.Printf("[Simulated Gateway] Cannot revert payment for Order %s. Status: %s (Exists: %t)", orderID, tx.Status, exists)
		return "failure", fmt.Errorf("payment for order %s is not completed or does not exist", orderID)
	}

	if rand.Float32() < 0.10 {
		tx.Status = "failed"
		tx.Timestamp = time.Now()
		tx.ErrorReason = "random refund error"
		simulatedGatewayDB.Transactions[orderID] = tx
		log.Printf("[Simulated Gateway] Refund for Order %s FAILED (random error).", orderID)
		return "failure", fmt.Errorf("random refund error")
	}

	tx.Status = "refunded"
	tx.Timestamp = time.Now()
	tx.ErrorReason = reason
	simulatedGatewayDB.Transactions[orderID] = tx
	log.Printf("[Simulated Gateway] Payment for Order %s reverted.", orderID)
	return "success", nil
}
