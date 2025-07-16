package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/StitchMl/saga-demo/backend/choreographer_saga/shared"
	inventorydb "github.com/StitchMl/saga-demo/backend/common/data_store"
	"github.com/StitchMl/saga-demo/backend/common/types"
)

var eventBus *shared.EventBus

func main() {
	inventorydb.InitDB()

	rabbitMQURL := os.Getenv("RABBITMQ_URL")
	if rabbitMQURL == "" {
		log.Fatal("RABBITMQ_URL non impostata")
	}

	eventBus, err := shared.NewEventBus(rabbitMQURL)
	if err != nil {
		log.Fatalf("Impossibile creare EventBus: %v", err)
	}
	defer eventBus.Close()

	for event, handler := range map[events.EventType]func(interface{}){
		events.OrderCreatedEvent:    handleOrderCreatedEvent,
		events.RevertInventoryEvent: handleRevertInventoryEvent,
	} {
		if err := eventBus.Subscribe(event, handler); err != nil {
			log.Fatalf("Errore sottoscrizione %s: %v", event, err)
		}
	}

	http.HandleFunc("/products/prices", getProductPricesHandler)

	port := os.Getenv("INVENTORY_SERVICE_PORT")
	if port == "" {
		log.Fatal("INVENTORY_SERVICE_PORT non impostata")
	}
	log.Printf("Inventory Service avviato su porta %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func getProductPricesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	productIDs := r.URL.Query()["product_id"]
	if len(productIDs) == 0 {
		http.Error(w, "Missing 'product_id' query parameter(s)", http.StatusBadRequest)
		return
	}

	prices := make(map[string]float64, len(productIDs))
	for _, id := range productIDs {
		if price, ok := inventorydb.GetProductPrice(id); ok {
			prices[id] = price
		} else {
			http.Error(w, fmt.Sprintf("Product ID %s not found", id), http.StatusNotFound)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(prices)
}

// handleOrderCreatedEvent handles the order creation request
func handleOrderCreatedEvent(eventPayload interface{}) {
	var payload events.OrderCreatedPayload
	if err := mapToStruct(eventPayload, &payload); err != nil {
		log.Printf("Inventory Service: Payload error: %v", err)
		return
	}

	log.Printf("Inventory Service: Received OrderCreatedEvent %s for %d items", payload.OrderID, len(payload.Items))

	inventorydb.DB.Inventory.Lock()
	defer inventorydb.DB.Inventory.Unlock()

	reservedItems := make([]events.OrderItem, 0, len(payload.Items))
	for _, item := range payload.Items {
		available, exists := inventorydb.DB.Inventory.Data[item.ProductID]
		if !exists || available < item.Quantity {
			// Ripristina eventuali riserve
			for _, r := range reservedItems {
				inventorydb.DB.Inventory.Data[r.ProductID] += r.Quantity
			}
			_ = eventBus.Publish(events.NewGenericEvent(
				events.InventoryReservationFailedEvent,
				payload.OrderID,
				"Inventory reservation failed",
				events.InventoryReservationFailedPayload{
					OrderID:   payload.OrderID,
					ProductID: item.ProductID,
					Quantity:  item.Quantity,
					Reason:    "Insufficient quantity or product not found",
				},
			))
			return
		}
		inventorydb.DB.Inventory.Data[item.ProductID] -= item.Quantity
		reservedItems = append(reservedItems, item)
	}

	_ = eventBus.Publish(events.NewGenericEvent(
		events.InventoryReservedEvent,
		payload.OrderID,
		"Inventory reserved successfully",
		events.InventoryReservedPayload{
			OrderID: payload.OrderID,
			Items:   reservedItems,
		},
	))
}

// mapToStruct performs a generic conversion from an interface{} to struct via JSON.
func mapToStruct(src interface{}, dst interface{}) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

// handleRevertInventoryEvent handles the inventory clearing request
func handleRevertInventoryEvent(eventPayload interface{}) {
	var payload events.OrderCreatedPayload
	if err := mapToStruct(eventPayload, &payload); err != nil {
		log.Printf("Inventory Service: Payload error: %v", err)
		return
	}

	inventorydb.DB.Inventory.Lock()
	defer inventorydb.DB.Inventory.Unlock()

	for _, item := range payload.Items {
		inventorydb.DB.Inventory.Data[item.ProductID] += item.Quantity
	}
	log.Printf("Inventory Service: Restored %d items for Order %s.", len(payload.Items), payload.OrderID)
}
