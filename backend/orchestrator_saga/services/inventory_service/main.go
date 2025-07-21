package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	events "github.com/StitchMl/saga-demo/common/types"
)

const (
	contentTypeJSON       = "application/json"
	contentType           = "Content-Type"
	errorMethodNotAllowed = "Metodo non consentito"
)

// ProductsDB simula un database di prodotti.
var ProductsDB = struct {
	sync.RWMutex
	Data map[string]events.Product
}{Data: make(map[string]events.Product)}

func initDB() {
	ProductsDB.Lock()
	defer ProductsDB.Unlock()

	ProductsDB.Data = map[string]events.Product{
		"prod-1": {ID: "prod-1", Name: "Laptop Pro", Description: "Un laptop potente per professionisti.", Price: 1299.99, Available: 100, ImageURL: "https://placehold.co/600x400/595964/FFF?text=Laptop"},
		"prod-2": {ID: "prod-2", Name: "Mouse Wireless", Description: "Mouse ergonomico e preciso.", Price: 49.50, Available: 50, ImageURL: "https://placehold.co/600x400/595964/FFF?text=Mouse"},
		"prod-3": {ID: "prod-3", Name: "Tastiera Meccanica", Description: "Tastiera con switch meccanici per gaming.", Price: 120.00, Available: 200, ImageURL: "https://placehold.co/600x400/595964/FFF?text=Tastiera"},
	}
	log.Println("[ServizioInventario] Database in-memoria inizializzato.")
}

func main() {
	port := os.Getenv("INVENTORY_SERVICE_PORT")
	if port == "" {
		log.Fatal("Variabile d'ambiente INVENTORY_SERVICE_PORT non impostata.")
	}
	initDB()
	http.HandleFunc("/reserve", reserveInventoryHandler)
	http.HandleFunc("/cancel_reservation", cancelReservationHandler)
	http.HandleFunc("/catalog", catalogHandler)
	http.HandleFunc("/get_price", getPriceHandler) // Nuovo endpoint per i prezzi
	log.Printf("Servizio Inventario avviato sulla porta %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// getPriceHandler restituisce il prezzo di un singolo prodotto.
func getPriceHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, errorMethodNotAllowed, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ProductID string `json:"product_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Corpo della richiesta non valido", http.StatusBadRequest)
		return
	}

	ProductsDB.RLock()
	product, ok := ProductsDB.Data[req.ProductID]
	ProductsDB.RUnlock()

	if !ok {
		http.Error(w, "Prodotto non trovato", http.StatusNotFound)
		return
	}

	w.Header().Set(contentType, contentTypeJSON)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"product_id": req.ProductID,
		"price":      fmt.Sprintf("%.2f", product.Price),
		"status":     "success",
	})
}

// catalogHandler gestisce le richieste per ottenere il catalogo prodotti.
func catalogHandler(w http.ResponseWriter, _ *http.Request) {
	ProductsDB.RLock()
	defer ProductsDB.RUnlock()

	list := make([]events.Product, 0, len(ProductsDB.Data))
	for _, p := range ProductsDB.Data {
		list = append(list, p)
	}
	w.Header().Set(contentType, contentTypeJSON)
	_ = json.NewEncoder(w).Encode(list)
}

// reserveInventoryHandler gestisce la prenotazione dei prodotti.
func reserveInventoryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, errorMethodNotAllowed, http.StatusMethodNotAllowed)
		return
	}

	var req events.InventoryRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Corpo della richiesta non valido", http.StatusBadRequest)
		return
	}

	ProductsDB.Lock()
	defer ProductsDB.Unlock()

	// Controlla la disponibilità prima di apportare modifiche
	for _, item := range req.Items {
		product, ok := ProductsDB.Data[item.ProductID]
		if !ok || product.Available < item.Quantity {
			w.Header().Set(contentType, contentTypeJSON)
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status":  "error",
				"message": "Quantità insufficiente o prodotto non trovato per " + item.ProductID,
			})
			return
		}
	}

	// Prenota gli articoli
	for _, item := range req.Items {
		product := ProductsDB.Data[item.ProductID]
		product.Available -= item.Quantity
		ProductsDB.Data[item.ProductID] = product
	}

	log.Printf("Inventario prenotato per l'Ordine %s", req.OrderID)
	w.Header().Set(contentType, contentTypeJSON)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Inventario prenotato"})
}

// cancelReservationHandler gestisce l'annullamento di una prenotazione (compensazione).
func cancelReservationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, errorMethodNotAllowed, http.StatusMethodNotAllowed)
		return
	}

	var req events.InventoryRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Richiesta non valida", http.StatusBadRequest)
		return
	}

	ProductsDB.Lock()
	defer ProductsDB.Unlock()

	for _, item := range req.Items {
		product := ProductsDB.Data[item.ProductID]
		product.Available += item.Quantity
		ProductsDB.Data[item.ProductID] = product
	}

	log.Printf("Annullata prenotazione inventario per l'Ordine %s", req.OrderID)
	w.Header().Set(contentType, contentTypeJSON)
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Prenotazione annullata e inventario ripristinato",
	}); err != nil {
		printEncodeError(err, w)
	}
}

func printEncodeError(err error, w http.ResponseWriter) {
	log.Printf("Errore nella codifica della risposta: %v", err)
	http.Error(w, "Errore interno del server", http.StatusInternalServerError)
}
