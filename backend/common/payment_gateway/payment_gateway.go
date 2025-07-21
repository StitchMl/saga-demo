package payment_gateway

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"time"
)

// --------------------------------------------------------------------
//  Database in-memoria del gateway – condiviso da entrambe le modalità
// --------------------------------------------------------------------

var (
	simulatedGatewayDB = struct {
		sync.RWMutex
		Transactions map[string]string // OrderID -> Status
	}{Transactions: make(map[string]string)}

	// Configurable parameters
	paymentAmountLimit float64
	randomFailureRate  float64
)

func init() {
	rand.New(rand.NewSource(time.Now().UnixNano()))

	limitStr := os.Getenv("PAYMENT_GATEWAY_LIMIT")
	if limitStr == "" {
		limitStr = "2000.0" // More realistic default limit
	}
	var err error
	paymentAmountLimit, err = strconv.ParseFloat(limitStr, 64)
	if err != nil {
		log.Fatalf("Invalid value for PAYMENT_GATEWAY_LIMIT: %v", err)
	}

	rateStr := os.Getenv("PAYMENT_GATEWAY_FAILURE_RATE")
	if rateStr == "" {
		rateStr = "0.15" // 15% random failures by default
	}
	randomFailureRate, err = strconv.ParseFloat(rateStr, 64)
	if err != nil {
		log.Fatalf("Invalid value for PAYMENT_GATEWAY_FAILURE_RATE: %v", err)
	}

	log.Println("[Simulated Payment Gateway] Initialised in memory.")
}

// --------------------------------------------------------------------
//  Public APIs used by Payment Microservices
// --------------------------------------------------------------------

// ProcessPayment simulates the processing of a payment. It returns an error if failure.
func ProcessPayment(orderID, customerID string, amount float64) error {
	if orderID == "" || customerID == "" {
		return fmt.Errorf("orderID o customerID mancanti")
	}

	// Idempotence
	simulatedGatewayDB.Lock()
	if status, ok := simulatedGatewayDB.Transactions[orderID]; ok && status == "completed" {
		simulatedGatewayDB.Unlock()
		return nil // Success, already processed
	}
	simulatedGatewayDB.Transactions[orderID] = "pending"
	simulatedGatewayDB.Unlock()

	time.Sleep(time.Duration(50+rand.Intn(150)) * time.Millisecond)

	// Bankruptcy checks
	if amount > paymentAmountLimit {
		return updateAndReturnError(orderID, fmt.Sprintf("amount %.2f exceeds the limit of %.2f", amount, paymentAmountLimit))
	}

	if rand.Float64() < randomFailureRate {
		reasons := []string{"insufficient funds', 'card rejected', 'generic gateway error"}
		reason := reasons[rand.Intn(len(reasons))]
		return updateAndReturnError(orderID, reason)
	}

	// Success
	simulatedGatewayDB.Lock()
	simulatedGatewayDB.Transactions[orderID] = "completed"
	simulatedGatewayDB.Unlock()
	return nil
}

// RevertPayment simulates the reimbursement/return of a payment.
func RevertPayment(orderID, reason string) error {
	simulatedGatewayDB.Lock()
	defer simulatedGatewayDB.Unlock()

	txStatus, ok := simulatedGatewayDB.Transactions[orderID]
	if !ok || txStatus != "completed" {
		log.Printf("[Simulated Payment Gateway] No completed payment to be reversed for order %s. Status: %s. Reason: %s", orderID, txStatus, reason)
		// This is not a mistake, there is simply nothing to reverse.
		return nil
	}

	time.Sleep(time.Duration(30+rand.Intn(70)) * time.Millisecond)

	if rand.Float64() < 0.05 { // Lower reimbursement failure rate
		simulatedGatewayDB.Transactions[orderID] = "failed_refund"
		return fmt.Errorf("random error during reimbursement")
	}

	simulatedGatewayDB.Transactions[orderID] = "refunded"
	return nil
}

// --------------------------------------------------------------------
//  Internal helper
// --------------------------------------------------------------------

// updateAndReturnError updates the transaction status to 'failed' and returns a formatted error.
func updateAndReturnError(orderID, reason string) error {
	simulatedGatewayDB.Lock()
	defer simulatedGatewayDB.Unlock()
	simulatedGatewayDB.Transactions[orderID] = "failed"
	return fmt.Errorf(reason)
}
