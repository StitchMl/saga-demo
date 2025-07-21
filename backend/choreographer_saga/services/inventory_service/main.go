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

const payloadErrorLogFmt = "Inventory Service: Error in payload: %v"

var eventBus *shared.EventBus

func main() {
	inventorydb.InitDB()

	rabbitMQURL := os.Getenv("RABBITMQ_URL")
	if rabbitMQURL == "" {
		log.Fatal("RABBITMQ_URL non impostata")
	}

	var err error
	eventBus, err = shared.NewEventBus(rabbitMQURL)
	if err != nil {
		log.Fatalf("Unable to create EventBus: %v", err)
	}
	defer eventBus.Close()

	subscribe(events.OrderCreatedEvent, handleOrderCreatedEvent)
	subscribe(events.RevertInventoryEvent, handleRevertInventoryEvent)

	http.HandleFunc("/products/prices", getProductPricesHandler)
	http.HandleFunc("/catalog", catalogHandler)

	port := os.Getenv("INVENTORY_SERVICE_PORT")
	if port == "" {
		log.Fatal("INVENTORY_SERVICE_PORT non impostata")
	}
	log.Printf("Inventory service started on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func subscribe(t events.EventType, h shared.EventHandler) {
	if err := eventBus.Subscribe(t, h); err != nil {
		log.Fatalf("Subscription error %s: %v", t, err)
	}
}

// ---------- Gestori Eventi ----------

// handleOrderCreatedEvent handles the order creation request
func handleOrderCreatedEvent(event events.GenericEvent) {
	var payload events.OrderCreatedPayload
	if err := mapToStruct(event.Payload, &payload); err != nil {
		log.Printf(payloadErrorLogFmt, err)
		return
	}

	log.Printf("Inventory Service: Received OrderCreatedEvent %s for %d items", payload.OrderID, len(payload.Items))

	inventorydb.DB.Products.Lock()
	defer inventorydb.DB.Products.Unlock()

	var totalAmount float64
	// First, calculate the total and check the prices
	for i := range payload.Items {
		product, ok := inventorydb.DB.Products.Data[payload.Items[i].ProductID]
		if !ok {
			publishFailure(payload.OrderID, "Product price not found for "+payload.Items[i].ProductID, nil)
			return
		}
		payload.Items[i].Price = product.Price
		totalAmount += product.Price * float64(payload.Items[i].Quantity)
	}

	// Then, check availability and book
	reservedItems := make([]events.OrderItem, 0, len(payload.Items))
	for _, item := range payload.Items {
		product := inventorydb.DB.Products.Data[item.ProductID]
		if product.Available < item.Quantity {
			// Restore any reserves
			for _, r := range reservedItems {
				p := inventorydb.DB.Products.Data[r.ProductID]
				p.Available += r.Quantity
				inventorydb.DB.Products.Data[r.ProductID] = p
			}
			publishFailure(payload.OrderID, "Insufficient quantity for "+item.ProductID, &totalAmount)
			return
		}
		product.Available -= item.Quantity
		inventorydb.DB.Products.Data[item.ProductID] = product
		reservedItems = append(reservedItems, item)
	}

	publish(events.InventoryReservedEvent, payload.OrderID, "Booked inventory",
		events.InventoryRequestPayload{
			OrderID:    payload.OrderID,
			CustomerID: payload.CustomerID,
			Items:      reservedItems,
			Amount:     totalAmount,
		},
	)
}

// handleRevertInventoryEvent manages the inventory reversal request
func handleRevertInventoryEvent(event events.GenericEvent) {
	var payload events.InventoryRequestPayload
	if err := mapToStruct(event.Payload, &payload); err != nil {
		log.Printf(payloadErrorLogFmt, err)
		return
	}

	inventorydb.DB.Products.Lock()
	defer inventorydb.DB.Products.Unlock()

	for _, item := range payload.Items {
		if product, ok := inventorydb.DB.Products.Data[item.ProductID]; ok {
			product.Available += item.Quantity
			inventorydb.DB.Products.Data[item.ProductID] = product
		}
	}
	log.Printf("Inventory Service: Restored %d items for Order %s.", len(payload.Items), payload.OrderID)
}

// publishFailure is a helper to publish a booking failure event.
func publishFailure(orderID, reason string, total *float64) {
	payload := events.OrderStatusUpdatePayload{
		OrderID: orderID,
		Reason:  reason,
	}
	if total != nil {
		payload.Total = *total
	}
	publish(events.InventoryReservationFailedEvent, orderID, "Inventory reservation failed", payload)
}

// publish is a helper to publish an event.
func publish(t events.EventType, id, msg string, pl events.EventPayload) {
	if err := eventBus.Publish(events.NewGenericEvent(t, id, msg, pl)); err != nil {
		log.Printf("publication %s: %v", t, err)
	}
}

// ---------- Handler HTTP ----------

// catalogHandler handles requests to get the product catalog
func catalogHandler(w http.ResponseWriter, _ *http.Request) {
	inventorydb.DB.Products.RLock()
	defer inventorydb.DB.Products.RUnlock()

	list := make([]events.Product, 0, len(inventorydb.DB.Products.Data))
	for _, p := range inventorydb.DB.Products.Data {
		list = append(list, p)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

// getProductPricesHandler manages requests to obtain product prices
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
