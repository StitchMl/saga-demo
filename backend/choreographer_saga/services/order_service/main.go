package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/StitchMl/saga-demo/choreographer_saga/shared"
	inventorydb "github.com/StitchMl/saga-demo/common/data_store"
	events "github.com/StitchMl/saga-demo/common/types"
)

const (
	contentTypeJSON = "application/json"
	contentType     = "Content-Type"
)

var (
	eventBus           *shared.EventBus
	paymentAmountLimit float64
)

func main() {
	// Initialise the global data store
	inventorydb.InitDB()

	rabbitMQURL := os.Getenv("RABBITMQ_URL")
	port := os.Getenv("ORDER_SERVICE_PORT")
	if rabbitMQURL == "" || port == "" {
		log.Fatal("Missing env: RABBITMQ_URL, ORDER_SERVICE_PORT")
	}

	limitStr := os.Getenv("PAYMENT_AMOUNT_LIMIT")
	if limitStr == "" {
		log.Fatal("PAYMENT_AMOUNT_LIMIT not set")
	}
	var err error
	paymentAmountLimit, err = strconv.ParseFloat(limitStr, 64)
	if err != nil {
		log.Fatalf("Invalid PAYMENT_AMOUNT_LIMIT: %v", err)
	}

	eventBus, err = shared.NewEventBus(rabbitMQURL)
	if err != nil {
		log.Fatalf("Unable to create EventBus: %v", err)
	}
	defer eventBus.Close()

	// Subscriptions
	subscribe(events.PaymentProcessedEvent, handleOrderApprovedEvent)
	subscribe(events.PaymentFailedEvent, handlePaymentFailedEvent)
	subscribe(events.InventoryReservationFailedEvent, handleInventoryReservationFailed)

	// REST endpoints
	http.HandleFunc("/create_order", createOrderHandler)
	http.HandleFunc("/orders/", getOrderHandler)
	http.HandleFunc("/orders", listOrdersHandler)
	http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Choreographer Order Service OK"))
	})

	log.Printf("Choreographer Order Service listening on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// subscribe: utility to subscribe to events with error handling
func subscribe(t events.EventType, h shared.EventHandler) {
	if err := eventBus.Subscribe(t, h); err != nil {
		log.Fatalf("Subscription error %s: %v", t, err)
	}
}

// listOrdersHandler: returns all orders
func listOrdersHandler(w http.ResponseWriter, r *http.Request) {
	cid := r.URL.Query().Get("customer_id")

	inventorydb.DB.Orders.RLock()
	out := make([]events.Order, 0, len(inventorydb.DB.Orders.Data))
	for _, o := range inventorydb.DB.Orders.Data {
		if cid == "" || o.CustomerID == cid {
			out = append(out, o)
		}
	}
	inventorydb.DB.Orders.RUnlock()

	w.Header().Set(contentType, contentTypeJSON)
	_ = json.NewEncoder(w).Encode(out)
}

// getOrderHandler: retrieves a single order
func getOrderHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/orders/")
	inventorydb.DB.Orders.RLock()
	defer inventorydb.DB.Orders.RUnlock()
	if order, ok := inventorydb.DB.Orders.Data[id]; ok {
		w.Header().Set(contentType, contentTypeJSON)
		_ = json.NewEncoder(w).Encode(order)
		return
	}
	http.Error(w, "order not found", http.StatusNotFound)
}

// createOrderHandler: create the PENDING order and publish the Saga start event.
func createOrderHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var order events.Order
	if err := json.NewDecoder(r.Body).Decode(&order); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Synchronous pre-check: calculate total and check against payment limit
	var totalAmount float64
	for _, item := range order.Items {
		price, ok := inventorydb.GetProductPrice(item.ProductID)
		if !ok {
			http.Error(w, "Product price not found for "+item.ProductID, http.StatusBadRequest)
			return
		}
		totalAmount += price * float64(item.Quantity)
	}

	if totalAmount > paymentAmountLimit {
		reason := fmt.Sprintf("L'importo %.2f supera il limite di %.2f", totalAmount, paymentAmountLimit)
		w.Header().Set(contentType, contentTypeJSON)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": reason})
		return
	}

	order.OrderID = fmt.Sprintf("order-%d", time.Now().UnixNano())
	order.Status = "pending"

	// *** WRITING in the shared data store ***
	inventorydb.DB.Orders.Lock()
	inventorydb.DB.Orders.Data[order.OrderID] = order
	inventorydb.DB.Orders.Unlock()

	payload := events.OrderCreatedPayload{
		OrderID:    order.OrderID,
		Items:      order.Items,
		CustomerID: order.CustomerID,
	}

	if err := eventBus.Publish(
		events.NewGenericEvent(events.OrderCreatedEvent, order.OrderID, "New order created", payload),
	); err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"message":  "Order received, SAGA initiated",
		"order_id": order.OrderID,
	})
}

// handleOrderApprovedEvent: update status -> approved
func handleOrderApprovedEvent(event events.GenericEvent) {
	var payload events.PaymentPayload
	if err := mapToStruct(event.Payload, &payload); err != nil {
		log.Printf("Order Service: Payload error OrderApprovedEvent: %v", err)
		return
	}
	updateOrderStatus(payload.OrderID, "approved", "Payment successful", &payload.Amount)
}

// handlePaymentFailedEvent: update status to rejected and trigger compensation
func handlePaymentFailedEvent(event events.GenericEvent) {
	var payload events.OrderStatusUpdatePayload
	if err := mapToStruct(event.Payload, &payload); err != nil {
		log.Printf("Order Service: Payload error for PaymentFailedEvent: %v", err)
		return
	}
	log.Printf("Order Service: Received PaymentFailedEvent for order %s. Reason: %s", payload.OrderID, payload.Reason)
	updateOrderStatus(payload.OrderID, "rejected", payload.Reason, &payload.Total)

	// Trigger inventory compensation
	if order, ok := inventorydb.GetOrder(payload.OrderID); ok {
		revertPayload := events.InventoryRequestPayload{
			OrderID: order.OrderID,
			Items:   order.Items,
			Reason:  "Payment failed, reverting inventory reservation.",
		}
		if err := eventBus.Publish(events.NewGenericEvent(events.RevertInventoryEvent, order.OrderID, "Reverting inventory", revertPayload)); err != nil {
			log.Printf("Order Service: Failed to publish RevertInventoryEvent for order %s: %v", order.OrderID, err)
		}
	}
}

// handleInventoryReservationFailed: update status → rejected
func handleInventoryReservationFailed(event events.GenericEvent) {
	var payload events.OrderStatusUpdatePayload
	if err := mapToStruct(event.Payload, &payload); err != nil {
		log.Printf("Order Service: Payload error for InventoryReservationFailedEvent: %v", err)
		return
	}
	log.Printf("Order Service: Received InventoryReservationFailedEvent for order %s. Reason: %s", payload.OrderID, payload.Reason)
	updateOrderStatus(payload.OrderID, "rejected", payload.Reason, &payload.Total)
}

// updateOrderStatus is a helper to change the order status in the DB.
func updateOrderStatus(orderID, status, reason string, total *float64) {
	inventorydb.DB.Orders.Lock()
	defer inventorydb.DB.Orders.Unlock()

	if order, exists := inventorydb.DB.Orders.Data[orderID]; exists {
		order.Status = status
		order.Reason = reason // Store the reason
		if total != nil {
			order.Total = *total
		}
		inventorydb.DB.Orders.Data[orderID] = order
		log.Printf("Order Service: Order %s status updated to %s. Reason: %s", orderID, status, reason)
	} else {
		log.Printf("Order Service: Order %s not found for status update.", orderID)
	}
}

// mapToStruct: utility to convert a generic payload into a specific struct.
func mapToStruct(src, dst interface{}) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}
