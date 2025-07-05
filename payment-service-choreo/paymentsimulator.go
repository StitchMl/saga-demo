package main

import (
	"fmt"
	"strings"
	"time"
)

// PaymentSimulator defines the connection for an external payment simulator.
type PaymentSimulator interface {
	ProcessPayment(orderID string, amount float64) error
}

// externalPaymentSimulator implementa PaymentSimulator.
type externalPaymentSimulator struct {
	// Any configuration fields for the simulator (for example, failure rate)
}

// NewExternalPaymentSimulator creates a new instance of the simulator.
func NewExternalPaymentSimulator() PaymentSimulator {
	return &externalPaymentSimulator{}
}

// ProcessPayment simulates the processing of a payment with an external service.
// It can be configured to fail under certain conditions to test saga compensations.
func (s *externalPaymentSimulator) ProcessPayment(orderID string, amount float64) error {
	// Simulates network latency or processing time.
	time.Sleep(50 * time.Millisecond)

	// Simulates a failure condition if the orderID contains 'FAIL_PAYMENT'.
	// This allows explicit testing of failure paths.
	if strings.Contains(orderID, "FAIL_PAYMENT") {
		return fmt.Errorf("simulated external payment failure for order %s: insufficient funds or declined", orderID)
	}

	// Simulates a failure if the amount is zero or negative.
	if amount <= 0 {
		return fmt.Errorf("invalid payment amount: %.2f", amount)
	}

	// Otherwise, payment is simulated as successful.
	return nil
}
