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

const payloadErrorLogFmt = "Servizio Inventario: Errore nel payload: %v"

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
	log.Printf("Servizio Inventario avviato sulla porta %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func subscribe(t events.EventType, h shared.EventHandler) {
	if err := eventBus.Subscribe(t, h); err != nil {
		log.Fatalf("Errore di sottoscrizione %s: %v", t, err)
	}
}

// ---------- Gestori Eventi ----------

// handleOrderCreatedEvent gestisce la richiesta di creazione ordine
func handleOrderCreatedEvent(event events.GenericEvent) {
	var payload events.OrderCreatedPayload
	if err := mapToStruct(event.Payload, &payload); err != nil {
		log.Printf(payloadErrorLogFmt, err)
		return
	}

	log.Printf("Servizio Inventario: Ricevuto OrderCreatedEvent %s per %d articoli", payload.OrderID, len(payload.Items))

	inventorydb.DB.Products.Lock()
	defer inventorydb.DB.Products.Unlock()

	var totalAmount float64
	// Per prima cosa, calcola il totale e controlla i prezzi
	for i := range payload.Items {
		product, ok := inventorydb.DB.Products.Data[payload.Items[i].ProductID]
		if !ok {
			publishFailure(payload.OrderID, "Prezzo del prodotto non trovato per "+payload.Items[i].ProductID, nil)
			return
		}
		payload.Items[i].Price = product.Price
		totalAmount += product.Price * float64(payload.Items[i].Quantity)
	}

	// Poi, controlla la disponibilità e prenota
	reservedItems := make([]events.OrderItem, 0, len(payload.Items))
	for _, item := range payload.Items {
		product := inventorydb.DB.Products.Data[item.ProductID]
		if product.Available < item.Quantity {
			// Ripristina eventuali riserve
			for _, r := range reservedItems {
				p := inventorydb.DB.Products.Data[r.ProductID]
				p.Available += r.Quantity
				inventorydb.DB.Products.Data[r.ProductID] = p
			}
			publishFailure(payload.OrderID, "Quantità insufficiente per "+item.ProductID, &totalAmount)
			return
		}
		product.Available -= item.Quantity
		inventorydb.DB.Products.Data[item.ProductID] = product
		reservedItems = append(reservedItems, item)
	}

	publish(events.InventoryReservedEvent, payload.OrderID, "Inventario prenotato",
		events.InventoryRequestPayload{
			OrderID:    payload.OrderID,
			CustomerID: payload.CustomerID,
			Items:      reservedItems,
			Amount:     totalAmount,
		},
	)
}

// handleRevertInventoryEvent gestisce la richiesta di storno dell'inventario
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
	log.Printf("Servizio Inventario: Ripristinati %d articoli per l'Ordine %s.", len(payload.Items), payload.OrderID)
}

// publishFailure è un helper per pubblicare un evento di fallimento della prenotazione.
func publishFailure(orderID, reason string, total *float64) {
	payload := events.OrderStatusUpdatePayload{
		OrderID: orderID,
		Reason:  reason,
	}
	if total != nil {
		payload.Total = *total
	}
	publish(events.InventoryReservationFailedEvent, orderID, "Prenotazione inventario fallita", payload)
}

// publish è un helper per pubblicare un evento.
func publish(t events.EventType, id, msg string, pl events.EventPayload) {
	if err := eventBus.Publish(events.NewGenericEvent(t, id, msg, pl)); err != nil {
		log.Printf("pubblicazione %s: %v", t, err)
	}
}

// ---------- Handler HTTP ----------

// catalogHandler gestisce le richieste per ottenere il catalogo prodotti
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

// getProductPricesHandler gestisce le richieste per ottenere i prezzi dei prodotti
func getProductPricesHandler(w http.ResponseWriter, r *http.Request) {
	productID := r.URL.Query().Get("id")
	if price, ok := inventorydb.GetProductPrice(productID); ok {
		_ = json.NewEncoder(w).Encode(map[string]float64{"price": price})
		return
	}
	http.Error(w, "prodotto non trovato", http.StatusNotFound)
}

// mapToStruct esegue una conversione generica da interface{} a struct tramite JSON.
func mapToStruct(src interface{}, dst interface{}) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}
