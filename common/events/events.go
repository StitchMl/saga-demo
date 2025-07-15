package events

import "time"

// EventType defines the type of subscription event
type EventType string

const (
	OrderCreatedEvent               EventType = "OrderCreated"
	InventoryReservedEvent          EventType = "InventoryReserved"
	InventoryReservationFailedEvent EventType = "InventoryReservationFailed"
	PaymentProcessedEvent           EventType = "PaymentProcessed"
	PaymentFailedEvent              EventType = "PaymentFailed"
	OrderApprovedEvent              EventType = "OrderApproved"
	OrderRejectedEvent              EventType = "OrderRejected"
	RevertInventoryEvent            EventType = "RevertInventory" // Per compensazione
	RevertPaymentEvent              EventType = "RevertPayment"   // Per compensazione
)

// SagaEventBase provides fields common to all SAGA events
type SagaEventBase struct {
	OrderID   string    `json:"order_id"`
	Timestamp time.Time `json:"timestamp"`
	Type      EventType `json:"type"`
	Details   string    `json:"details,omitempty"`
}

type OrderItem struct {
	ProductID string  `json:"product_id"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price,omitempty"`
}

// Order represents an order in the system
type Order struct {
	OrderID    string      `json:"order_id"`
	Items      []OrderItem `json:"items"`
	CustomerID string      `json:"customer_id"`
	Status     string      `json:"status"` // Pending, approved, rejected
}

// OrderCreatedPayload Data for the OrderCreated event
type OrderCreatedPayload struct {
	OrderID    string      `json:"order_id"`
	Items      []OrderItem `json:"items"`
	CustomerID string      `json:"customer_id"`
}

// InventoryReservedPayload Data for InventoryReserved Event
type InventoryReservedPayload struct {
	OrderID    string      `json:"order_id"`
	CustomerID string      `json:"customer_id"`
	Items      []OrderItem `json:"items"`
	Success    bool        `json:"success"`
	Reason     string      `json:"reason,omitempty"`
}

// InventoryReservationFailedPayload Data for InventoryReservationFailed Event
type InventoryReservationFailedPayload struct {
	OrderID   string `json:"order_id"`
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
	Reason    string `json:"reason"`
}

// PaymentProcessedPayload Data for the PaymentProcessed event
type PaymentProcessedPayload struct {
	OrderID    string  `json:"order_id"`
	CustomerID string  `json:"customer_id"`
	Amount     float64 `json:"amount"`
	Success    bool    `json:"success"`
	Reason     string  `json:"reason,omitempty"`
}

// PaymentFailedPayload Data for the PaymentFailed event
type PaymentFailedPayload struct {
	OrderID string  `json:"order_id"`
	Amount  float64 `json:"amount"`
	Reason  string  `json:"reason"`
}

// OrderApprovedPayload Data for the OrderApproved event
type OrderApprovedPayload struct {
	OrderID string `json:"order_id"`
}

// OrderRejectedPayload Data for the OrderRejected event
type OrderRejectedPayload struct {
	OrderID string `json:"order_id"`
	Reason  string `json:"reason"`
}

// RevertInventoryPayload Data for RevertInventory event (clearing)
type RevertInventoryPayload struct {
	OrderID string      `json:"order_id"`
	Items   []OrderItem `json:"items"`
	Reason  string      `json:"reason"`
}

// RevertPaymentPayload Data for RevertPayment Event (clearing)
type RevertPaymentPayload struct {
	OrderID string `json:"order_id"`
	Reason  string `json:"reason"`
}

// GenericEvent wrapper for all event payloads
type GenericEvent struct {
	SagaEventBase
	Payload interface{} `json:"payload"`
}

type OrderConfirmedEvent struct {
	OrderID string `json:"order_id"`
	Success bool   `json:"success"`
	Reason  string `json:"reason,omitempty"`
}

// User represents a registered user.
type User struct {
	Username string `json:"username"`
	Password string `json:"password"` // In a real system, this would be hashed
}

// NewGenericEvent creates a new generic event with the basic data and the specific payload.
func NewGenericEvent(eventType EventType, orderID, details string, payload interface{}) GenericEvent {
	return GenericEvent{
		SagaEventBase: SagaEventBase{
			OrderID:   orderID,
			Timestamp: time.Now(),
			Type:      eventType,
			Details:   details,
		},
		Payload: payload,
	}
}
