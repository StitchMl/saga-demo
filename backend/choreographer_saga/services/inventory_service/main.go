package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/StitchMl/saga-demo/choreographer_saga/shared"
	inventorydb "github.com/StitchMl/saga-demo/common/data_store"
	events "github.com/StitchMl/saga-demo/common/types"
)

const payloadErrorLogFmt = "Inventory Service: Payload error: %v"

var eventBus *shared.EventBus

func main() {
	inventorydb.InitDB()

	rabbitMQURL := os.Getenv("RABBITMQ_URL")
	if rabbitMQURL == "" {
		log.Fatal("RABBITMQ_URL not set")
	}

	eventBus, err := shared.NewEventBus(rabbitMQURL)
	if err != nil {
		log.Fatalf("Unable to create EventBus: %v", err)
	}
	defer eventBus.Close()

	for event, handler := range map[events.EventType]func(interface{}){
		events.OrderCreatedEvent:               handleOrderCreatedEvent,
		events.RevertInventoryEvent:            handleRevertInventoryEvent,
		events.InventoryReservationFailedEvent: handleInventoryReservationFailed,
	} {
		if err := eventBus.Subscribe(event, handler); err != nil {
			log.Fatalf("Subscription error %s: %v", event, err)
		}
	}

	http.HandleFunc("/products/prices", getProductPricesHandler)
	http.HandleFunc("/catalog", catalogHandler)

	port := os.Getenv("INVENTORY_SERVICE_PORT")
	if port == "" {
		log.Fatal("INVENTORY_SERVICE_PORT not set")
	}
	log.Printf("Inventory Service started on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// ---------- Handlers ----------

// handleOrderCreatedEvent handles the order creation request
func handleOrderCreatedEvent(eventPayload interface{}) {
	var payload events.OrderCreatedPayload
	if err := mapToStruct(eventPayload, &payload); err != nil {
		log.Printf(payloadErrorLogFmt, err)
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
			OrderID:    payload.OrderID,
			CustomerID: payload.CustomerID,
			Items:      reservedItems,
		},
	))
}

// handleRevertInventoryEvent handles the inventory clearing request
func handleRevertInventoryEvent(eventPayload interface{}) {
	var payload events.OrderCreatedPayload
	if err := mapToStruct(eventPayload, &payload); err != nil {
		log.Printf(payloadErrorLogFmt, err)
		return
	}

	inventorydb.DB.Inventory.Lock()
	defer inventorydb.DB.Inventory.Unlock()

	for _, item := range payload.Items {
		inventorydb.DB.Inventory.Data[item.ProductID] += item.Quantity
	}
	log.Printf("Inventory Service: Restored %d items for Order %s.", len(payload.Items), payload.OrderID)
}

// handleInventoryReservationFailed handles the case where inventory reservation fails
func handleInventoryReservationFailed(eventPayload interface{}) {
	var payload events.InventoryReservationFailedPayload
	if err := mapToStruct(eventPayload, &payload); err != nil {
		log.Printf(payloadErrorLogFmt, err)
		return
	}
	log.Printf("Inventory reservation failed for Order %s, Product %s: %s",
		payload.OrderID, payload.ProductID, payload.Reason)
}

// ---------- Helpers ----------

// catalogHandler handles requests to get the product catalog
func catalogHandler(w http.ResponseWriter, _ *http.Request) {
	type Product struct {
		ID          string  `json:"id"`
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Price       float64 `json:"price"`
		Available   int     `json:"available"`
	}

	// In this example, collect data from DataStore in-memory
	inventorydb.DB.Inventory.RLock()
	inventorydb.DB.Prices.RLock()
	defer inventorydb.DB.Inventory.RUnlock()
	defer inventorydb.DB.Prices.RUnlock()

	list := make([]Product, 0, len(inventorydb.DB.Inventory.Data))
	for id, qty := range inventorydb.DB.Inventory.Data {
		price := inventorydb.DB.Prices.Data[id]
		list = append(list, Product{
			ID: id, Name: id, Description: "",
			Price: price, Available: qty,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

// getProductPricesHandler handles requests to get product prices
func getProductPricesHandler(w http.ResponseWriter, r *http.Request) {
	productID := r.URL.Query().Get("id")
	if price, ok := inventorydb.GetProductPrice(productID); ok {
		_ = json.NewEncoder(w).Encode(map[string]float64{"price": price})
		return
	}
	http.Error(w, "product not found", http.StatusNotFound)
}

// mapToStruct performs a generic conversion from an interface{} to struct via JSON.
func mapToStruct(src interface{}, dst interface{}) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}
