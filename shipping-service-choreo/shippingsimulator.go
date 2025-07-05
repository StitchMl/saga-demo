package main

import (
	"fmt"
	"strings"
	"time"
)

// ShippingSimulator defines the connection for an external shipping simulator.
type ShippingSimulator interface {
	ProcessShipping(orderID string) error
}

// externalShippingSimulator implementa ShippingSimulator.
type externalShippingSimulator struct {
	// Any configuration fields for the simulator (for example, failure rates)
}

// NewExternalShippingSimulator creates a new instance of the simulator.
func NewExternalShippingSimulator() ShippingSimulator {
	return &externalShippingSimulator{}
}

// ProcessShipping simulates the processing of a shipment with an external service.
// Can be configured to fail under certain conditions to test saga compensation.
func (s *externalShippingSimulator) ProcessShipping(orderID string) error {
	// Simulates network latency or processing time.
	time.Sleep(75 * time.Millisecond) // A little longer than payment

	// Simulates a failure condition if the orderID contains 'FAIL_SHIPMENT'.
	// This allows explicit testing of failure paths.
	if strings.Contains(orderID, "FAIL_SHIPMENT") {
		return fmt.Errorf("simulated external shipping failure for order %s: package damaged or address invalid", orderID)
	}

	// Otherwise, the shipment is simulated as successful.
	return nil
}
