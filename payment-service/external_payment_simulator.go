package main

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// PaymentSimulator defines the connection for an external payment simulator.
type PaymentSimulator interface {
	ProcessPayment(orderID string, amount float64) error
}

// externalPaymentSimulator implements PaymentSimulator.
type externalPaymentSimulator struct {
	// Any configuration fields for the simulator (for example, failure rate)
}

// NewExternalPaymentSimulator creates a new instance of the simulator.
func NewExternalPaymentSimulator() PaymentSimulator {
	// It only initializes the random number generator once.
	rand.New(rand.NewSource(time.Now().UnixNano()))
	return &externalPaymentSimulator{}
}

// ProcessPayment simulates the processing of a payment by an external gateway.
// To test saga compensation, the payment will fail if the orderID contains 'FAIL_PAYMENT'.
func (s *externalPaymentSimulator) ProcessPayment(orderID string, amount float64) error {
	// Simula la latenza di rete o il tempo di elaborazione.
	time.Sleep(50 * time.Millisecond)

	// Simulates a failure condition based on the order ID.
	// In a real scenario, this would depend on the actual response of the external service.
	if strings.Contains(orderID, "FAIL_PAYMENT") {
		return fmt.Errorf("simulated external payment failure for order %s: insufficient funds", orderID)
	}

	// Simulates another failure condition (for example, amount too low).
	if amount < 0.01 {
		return fmt.Errorf("payment amount %.2f is too low for processing", amount)
	}

	// Otherwise, payment is simulated as successful.
	return nil
}
