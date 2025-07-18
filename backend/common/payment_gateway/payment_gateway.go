package payment_gateway

import (
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	events "github.com/StitchMl/saga-demo/common/types"
)

// --------------------------------------------------------------------
//  In‑memory “gateway database” – condiviso da entrambe le modalità
// --------------------------------------------------------------------

var simulatedGatewayDB = struct {
	sync.RWMutex
	Transactions map[string]events.Transaction
}{Transactions: make(map[string]events.Transaction)}

func init() {
	rand.New(rand.NewSource(time.Now().UnixNano()))
	log.Println("[Simulated Gateway] In-memory gateway initialized.")
}

// --------------------------------------------------------------------
//  Public APIs used by PaymentService micro-services
// --------------------------------------------------------------------

// ProcessPayment simulates the processing of a payment.
func ProcessPayment(orderID, customerID string, amount float64) (string, error) {
	if orderID == "" || customerID == "" {
		return "failure", fmt.Errorf("orderID o customerID mancanti")
	}

	// idempotence
	simulatedGatewayDB.RLock()
	if tx, ok := simulatedGatewayDB.Transactions[orderID]; ok && tx.Status == "completed" {
		simulatedGatewayDB.RUnlock()
		return "success", nil
	}
	simulatedGatewayDB.RUnlock()

	addOrUpdateTx(orderID, customerID, amount, "pending", "")

	time.Sleep(time.Duration(50+rand.Intn(150)) * time.Millisecond)

	// random failure or by an amount > €100
	if rand.Float32() < 0.15 {
		return updateTxStatus(orderID, "failed", "random gateway error")
	}
	if amount > 100 {
		return updateTxStatus(orderID, "failed",
			fmt.Sprintf("amount %.2f exceeds limit", amount))
	}
	return updateTxStatus(orderID, "completed", "")
}

// RevertPayment simulates the repayment/compensation of a payment.
func RevertPayment(orderID, reason string) (string, error) {
	simulatedGatewayDB.RLock()
	tx, ok := simulatedGatewayDB.Transactions[orderID]
	simulatedGatewayDB.RUnlock()
	if !ok || tx.Status != "completed" {
		return "failure", fmt.Errorf("no completed payment for order %s", orderID)
	}

	time.Sleep(time.Duration(30+rand.Intn(70)) * time.Millisecond)
	if rand.Float32() < 0.10 {
		return updateTxStatus(orderID, "failed_refund", "random refund error")
	}
	return updateTxStatus(orderID, "refunded", reason)
}

// --------------------------------------------------------------------
//  Internal helper
// --------------------------------------------------------------------

func addOrUpdateTx(orderID, customerID string, amount float64, status, reason string) {
	simulatedGatewayDB.Lock()
	defer simulatedGatewayDB.Unlock()
	tx := simulatedGatewayDB.Transactions[orderID]
	tx.OrderID, tx.CustomerID, tx.Amount = orderID, customerID, amount
	tx.Status, tx.ErrorReason, tx.Timestamp = status, reason, time.Now()
	tx.Attempts++
	simulatedGatewayDB.Transactions[orderID] = tx
}

func updateTxStatus(orderID, status, reason string) (string, error) {
	addOrUpdateTx(orderID, "", 0, status, reason)
	if status == "completed" || status == "refunded" {
		return "success", nil
	}
	return "failure", fmt.Errorf(reason)
}
