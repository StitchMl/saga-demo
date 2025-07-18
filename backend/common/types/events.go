package events

import "time"

// EventType defines the type of subscription event
type EventType string

const (
	OrderCreatedEvent               EventType = "OrderCreated"
	InventoryReservedEvent          EventType = "InventoryReserved"
	InventoryReservationFailedEvent EventType = "InventoryReservationFailed"
	OrderApprovedEvent              EventType = "OrderApproved"
	OrderRejectedEvent              EventType = "OrderRejected"
	RevertInventoryEvent            EventType = "RevertInventory"

	UserRegisteredEvent EventType = "UserRegistered"
	UserLoginEvent      EventType = "UserLogin"
	ValidateEvent       EventType = "ValidateEvent"
)

// EventPayload is an interface to all event payloads, making their nature explicit.
type EventPayload interface{}

// BaseEvent provides fields common to all SAGA events.
type BaseEvent struct {
	OrderID   string    `json:"order_id"`
	Timestamp time.Time `json:"timestamp"`
	Type      EventType `json:"type"`
	Details   string    `json:"details,omitempty"`
}

// OrderItem represents an item within an order
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

// --- Payload of Events ---

// OrderCreatedPayload data for the OrderCreated event
type OrderCreatedPayload struct {
	OrderID    string      `json:"order_id"`
	Items      []OrderItem `json:"items"`
	CustomerID string      `json:"customer_id"`
}

// InventoryRequestPayload data for inventory request
// Reuse OrderItem for a list of items.
type InventoryRequestPayload struct {
	OrderID string      `json:"order_id"`
	Items   []OrderItem `json:"items"`
	Reason  string      `json:"reason,omitempty"`
}

// InventoryReservedPayload data for InventoryReserved event
type InventoryReservedPayload struct {
	OrderID    string      `json:"order_id"`
	CustomerID string      `json:"customer_id"`
	Items      []OrderItem `json:"items"`
	Success    bool        `json:"success"`
	Reason     string      `json:"reason,omitempty"`
}

// InventoryReservationFailedPayload data for the InventoryReservationFailed event
type InventoryReservationFailedPayload struct {
	OrderID   string `json:"order_id"`
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
	Reason    string `json:"reason"`
}

// PaymentPayload common data for PaymentProcessed and PaymentFailed
type PaymentPayload struct {
	OrderID    string  `json:"order_id"`
	CustomerID string  `json:"customer_id,omitempty"`
	Amount     float64 `json:"amount"`
	Success    bool    `json:"success"`
	Reason     string  `json:"reason,omitempty"`
}

// OrderStatusUpdatePayload Data for order status update events.
type OrderStatusUpdatePayload struct {
	OrderID string `json:"order_id"`
	Reason  string `json:"reason,omitempty"`
	Success bool   `json:"success,omitempty"`
}

// GenericEvent wrapper for all event payloads
type GenericEvent struct {
	BaseEvent
	Payload EventPayload `json:"payload"`
}

// NewGenericEvent creates a new generic event with the base data and the specific payload.
func NewGenericEvent(eventType EventType, orderID, details string, payload EventPayload) GenericEvent {
	return GenericEvent{
		BaseEvent: BaseEvent{
			OrderID:   orderID,
			Timestamp: time.Now(),
			Type:      eventType,
			Details:   details,
		},
		Payload: payload,
	}
}
