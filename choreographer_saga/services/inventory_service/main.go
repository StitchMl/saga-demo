package main

import (
	"encoding/json"
	"fmt"
	"github.com/StitchMl/saga-demo/choreographer_saga/shared"
	"github.com/StitchMl/saga-demo/common/events"
	inventorydb "github.com/StitchMl/saga-demo/common/inventory_db"
	"log"
	"net/http"
	"os"
)

var eventBus *shared.EventBus

func main() {
	// Inizializza il DB dell'inventario e dei prezzi
	inventorydb.InitDB()

	rabbitMQURL := os.Getenv("RABBITMQ_URL") // Default URL of RabbitMQ
	if rabbitMQURL == "" {
		log.Fatal("RABBITMQ_URL environment variable not set.")
	}

	var err error
	eventBus, err = shared.NewEventBus(rabbitMQURL)
	if err != nil {
		log.Fatalf("Order Service: Failed to create event bus: %v", err)
	}
	defer eventBus.Close() // Make sure to close the connection when the service stops

	// Subscribe to events relevant to the inventory service
	if err := eventBus.Subscribe(events.OrderCreatedEvent, handleOrderCreatedEvent); err != nil {
		log.Fatalf("Inventory Service: Failed to subscribe to OrderCreatedEvent: %v", err)
	}
	if err := eventBus.Subscribe(events.RevertInventoryEvent, handleRevertInventoryEvent); err != nil {
		log.Fatalf("Inventory Service: Failed to subscribe to RevertInventoryEvent: %v", err)
	}

	// HTTP endpoint to get product prices
	http.HandleFunc("/products/prices", getProductPricesHandler)

	// The inventory service in the choreography pattern has no HTTP endpoints for its main operations
	// (for example, /reserve, /cancel_reservation) but reacts to events.
	// It may have endpoints for status or administration.
	log.Println("Inventory Service started, listening for events and on port 8081...")
	log.Fatal(http.ListenAndServe(":8081", nil))
}

func getProductPricesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	productIDs := r.URL.Query()["product_id"] // Gets a list of product_id from the query string.
	if len(productIDs) == 0 {
		http.Error(w, "Missing 'product_id' query parameter(s)", http.StatusBadRequest)
		return
	}

	prices := make(map[string]float64)
	for _, id := range productIDs {
		price, ok := inventorydb.GetProductPrice(id)
		if ok {
			prices[id] = price
		} else {
			http.Error(w, fmt.Sprintf("Product ID %s not found", id), http.StatusNotFound)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(prices); err != nil {
		log.Printf("Inventory Service: Error encoding product prices response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// handleOrderCreatedEvent handles the order creation request
func handleOrderCreatedEvent(eventPayload interface{}) {
	payloadBytes, err := json.Marshal(eventPayload)
	if err != nil {
		log.Printf("Inventory Service: Error marshalling eventPayload to bytes: %v", err)
		return
	}

	var payload events.OrderCreatedPayload
	err = json.Unmarshal(payloadBytes, &payload)
	if err != nil {
		log.Printf("Inventory Service: Error unmarshalling payload bytes to OrderCreatedPayload: %v", err)
		return
	}

	log.Printf("Inventory Service: Received OrderCreatedEvent %s for Customer %s with %d items", payload.OrderID, payload.CustomerID, len(payload.Items))

	inventorydb.DB.Lock()
	defer inventorydb.DB.Unlock()

	var reservedItems []events.OrderItem
	// Itera su ogni articolo nell'ordine per tentare la riserva
	for _, item := range payload.Items {
		available, exists := inventorydb.DB.Data[item.ProductID]
		if !exists || available < item.Quantity {
			log.Printf("Inventory Service: Insufficient quantity for Order %s, Product %s. Available: %d, Required: %d", payload.OrderID, item.ProductID, available, item.Quantity)

			for _, reserved := range reservedItems {
				inventorydb.DB.Data[reserved.ProductID] += reserved.Quantity
				log.Printf("Inventory Service: Compensated %d units of Product %s for Order %s due to later item failure. Current inventory: %d", reserved.Quantity, reserved.ProductID, payload.OrderID, inventorydb.DB.Data[reserved.ProductID])
			}

			// Publish the reserve failure event
			failPayload := events.InventoryReservationFailedPayload{
				OrderID:   payload.OrderID,
				ProductID: item.ProductID,
				Quantity:  item.Quantity,
				Reason:    fmt.Sprintf("Insufficient quantity or product not found for product %s", item.ProductID),
			}
			if err := eventBus.Publish(events.NewGenericEvent(events.InventoryReservationFailedEvent, payload.OrderID, "Inventory reservation failed", failPayload)); err != nil {
				log.Printf("Inventory Service: Failed to publish InventoryReservationFailedEvent: %v", err)
			}
			return
		}

		inventorydb.DB.Data[item.ProductID] -= item.Quantity
		log.Printf("Inventory Service: Reserved %d units of Product %s for Order %s. Remaining inventory: %d", item.Quantity, item.ProductID, payload.OrderID, inventorydb.DB.Data[item.ProductID])

		reservedItems = append(reservedItems, item)
	}

	successPayload := events.InventoryReservedPayload{
		OrderID: payload.OrderID,
		Items:   reservedItems,
	}
	if err := eventBus.Publish(events.NewGenericEvent(events.InventoryReservedEvent, payload.OrderID, "Inventory reserved successfully", successPayload)); err != nil {
		log.Printf("Inventory Service: Failed to publish InventoryReservedEvent: %v", err)
	}
}

// handleRevertInventoryEvent handles the inventory clearing request
func handleRevertInventoryEvent(eventPayload interface{}) {
	payloadBytes, err := json.Marshal(eventPayload)
	if err != nil {
		log.Printf("Inventory Service: Error marshalling eventPayload to bytes: %v", err)
		return
	}

	var payload events.OrderCreatedPayload
	err = json.Unmarshal(payloadBytes, &payload)
	if err != nil {
		log.Printf("Inventory Service: Error unmarshalling payload bytes to OrderCreatedPayload: %v", err)
		return
	}

	log.Printf("Inventory Service: Received RevertInventoryEvent for Order %s, Quantity %d", payload.OrderID, len(payload.Items))

	inventorydb.DB.Lock()
	defer inventorydb.DB.Unlock()

	// Restore quantity in inventory
	for _, item := range payload.Items {
		inventorydb.DB.Data[item.ProductID] += item.Quantity
		log.Printf("Inventory Service: Restored %d units of Product %s for Order %s. New inventory: %d", item.Quantity, item.ProductID, payload.OrderID, inventorydb.DB.Data[item.ProductID])
	}
	log.Printf("Inventory Service: Restored %d units of Product for Order %s.", len(payload.Items), payload.OrderID)
}
