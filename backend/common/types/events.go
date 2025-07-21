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
	RevertInventoryEvent            EventType = "RevertInventory"
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
	Total      float64     `json:"total,omitempty"`
	Status     string      `json:"status"` // Pending, approved, rejected
	Reason     string      `json:"reason,omitempty"`
}

// Product defines the structure of a product.
type Product struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	Price       float64 `json:"price"`
	Available   int     `json:"available"`
	ImageURL    string  `json:"image_url,omitempty"`
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
	OrderID    string      `json:"order_id"`
	Items      []OrderItem `json:"items"`
	Reason     string      `json:"reason,omitempty"`
	Amount     float64     `json:"amount,omitempty"`
	CustomerID string      `json:"customer_id,omitempty"`
}

// PaymentPayload common data for PaymentProcessed and PaymentFailed
type PaymentPayload struct {
	OrderID    string  `json:"order_id"`
	CustomerID string  `json:"customer_id,omitempty"`
	Amount     float64 `json:"amount"`
	Reason     string  `json:"reason,omitempty"`
}

// OrderStatusUpdatePayload Data for order status update events.
type OrderStatusUpdatePayload struct {
	OrderID string  `json:"order_id"`
	Total   float64 `json:"total,omitempty"`
	Status  string  `json:"status"`
	Reason  string  `json:"reason,omitempty"`
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
