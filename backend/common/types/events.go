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

// Product definisce la struttura di un prodotto.
type Product struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	Price       float64 `json:"price"`
	Available   int     `json:"available"`
	ImageURL    string  `json:"image_url,omitempty"`
}

// --- Payload of Events ---

// OrderCreatedPayload dati per l'evento OrderCreated
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

// PaymentPayload dati comuni per PaymentProcessed e PaymentFailed
type PaymentPayload struct {
	OrderID    string  `json:"order_id"`
	CustomerID string  `json:"customer_id,omitempty"`
	Amount     float64 `json:"amount"`
	Reason     string  `json:"reason,omitempty"`
}

// OrderStatusUpdatePayload Dati per gli eventi di aggiornamento dello stato dell'ordine.
type OrderStatusUpdatePayload struct {
	OrderID string  `json:"order_id"`
	Total   float64 `json:"total,omitempty"`
	Status  string  `json:"status"`
	Reason  string  `json:"reason,omitempty"`
}

// GenericEvent wrapper per tutti i payload degli eventi
type GenericEvent struct {
	BaseEvent
	Payload EventPayload `json:"payload"`
}

// NewGenericEvent crea un nuovo evento generico con i dati di base e il payload specifico.
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
