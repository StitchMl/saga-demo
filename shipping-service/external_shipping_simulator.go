package main

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// ShippingSimulator defines the connection for an external shipping simulator.
type ShippingSimulator interface {
	ScheduleShipment(orderID string, address string) error
}

// externalShippingSimulator implements ShippingSimulator.
type externalShippingSimulator struct {
	// Any configuration fields for the simulator (for example, failure rate)
}

// NewExternalShippingSimulator creates a new instance of the simulator.
func NewExternalShippingSimulator() ShippingSimulator {
	// It only initializes the random number generator once.
	rand.New(rand.NewSource(time.Now().UnixNano()))
	return &externalShippingSimulator{}
}

// ScheduleShipment simulates the scheduling of a shipment with an external service.
// To test saga compensation, the shipment will fail if the orderID contains "FAIL_SHIPMENT".
// Or if the address is “Invalid Address”.
func (s *externalShippingSimulator) ScheduleShipment(orderID string, address string) error {
	// Simulates network latency or processing time.
	time.Sleep(70 * time.Millisecond) // Slightly longer latency.

	// Simulates a failure condition based on the order ID.
	// In a real scenario, this would depend on the actual response of the external service.
	if strings.Contains(orderID, "FAIL_SHIPMENT") {
		return fmt.Errorf("simulated external shipping failure for order %s: logistic error", orderID)
	}

	// Simulates a failure for an invalid address.
	if address == "Invalid Address" {
		return fmt.Errorf("invalid shipping address provided: %s", address)
	}

	// Otherwise, the shipment is simulated as successful.
	return nil
}
