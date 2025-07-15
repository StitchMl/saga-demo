package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time" // Necessario per generare OrderID unici
)

// Order represents an order.
// È importante che questa struct sia la stessa usata dall'Orchestrator per la richiesta iniziale.
type Order struct {
	OrderID    string `json:"order_id"`
	ProductID  string `json:"product_id"`
	Quantity   int    `json:"quantity"`
	CustomerID string `json:"customer_id"`
	Status     string `json:"status"` // Pending, approved, rejected
}

// In-memory database for orders
var ordersDB = struct {
	sync.RWMutex
	Data map[string]Order
}{Data: make(map[string]Order)}

func main() {
	// Aggiungi l'endpoint per la creazione iniziale dell'ordine
	http.HandleFunc("/create_order", createOrderHandler)
	// Mantieni l'endpoint per la conferma/aggiornamento dello stato dell'ordine
	http.HandleFunc("/confirm", confirmOrderHandler)

	// Aggiungi un endpoint di health check
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "Orchestrator Order Service is healthy!")
	})

	servicePort := os.Getenv("ORDER_SERVICE_PORT")
	if servicePort == "" {
		servicePort = "8083" // Usa una porta diversa per evitare conflitti e per chiarezza.
		// Verifica che questa sia la porta configurata nel tuo orchestrator_main
	}

	log.Printf("Orchestrator Order Service started on port %s", servicePort)
	log.Fatal(http.ListenAndServe(":"+servicePort, nil))
}

// createOrderHandler gestisce la richiesta iniziale di creazione ordine dall'Orchestrator.
func createOrderHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var orderRequest Order // Usa la stessa struct Order per la richiesta
	err := json.NewDecoder(r.Body).Decode(&orderRequest)
	if err != nil {
		log.Printf("Order Service: Invalid request body for create_order: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Genera un OrderID unico (o usa quello fornito dall'orchestrator se lo manda)
	// Per semplicità, qui lo generiamo noi se non c'è. L'orchestrator di solito lo crea.
	if orderRequest.OrderID == "" {
		orderRequest.OrderID = fmt.Sprintf("order-%d", time.Now().UnixNano())
	}
	orderRequest.Status = "pending" // Stato iniziale dell'ordine

	ordersDB.Lock()
	ordersDB.Data[orderRequest.OrderID] = orderRequest
	ordersDB.Unlock()

	log.Printf("Order Service: Created new order %s for Customer %s, Product %s, Quantity %d. Status: %s",
		orderRequest.OrderID, orderRequest.CustomerID, orderRequest.ProductID, orderRequest.Quantity, orderRequest.Status)

	// Rispondi all'orchestrator con l'ID dell'ordine e lo stato iniziale
	w.WriteHeader(http.StatusOK)
	response := map[string]string{
		"order_id": orderRequest.OrderID,
		"status":   orderRequest.Status,
		"message":  "Order created successfully",
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Order Service: Error encoding create_order response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// confirmOrderHandler gestisce la conferma o l'aggiornamento dello stato di un ordine.
// Questo endpoint sarà chiamato dall'Orchestrator per aggiornare lo stato finale (approvato/rifiutato).
func confirmOrderHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		OrderID string `json:"order_id"`
		Status  string `json:"status"` // "approved" or "rejected"
	}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ordersDB.Lock()
	defer ordersDB.Unlock()

	order, exists := ordersDB.Data[req.OrderID]
	if !exists {
		log.Printf("Order Service: Order %s not found for status update. Creating with status %s as a fallback.", req.OrderID, req.Status)
		// Questo caso dovrebbe essere raro se l'orchestrator ha già creato l'ordine.
		// Potrebbe accadere solo se un evento di compensazione arriva prima della creazione iniziale (problemi di tempistiche).
		// In un sistema robusto, qui si potrebbe voler loggare un warning o ritentare.
		order = Order{
			OrderID: req.OrderID,
			Status:  req.Status,
			// Altri campi non saranno popolati in questo "fallback creation"
		}
	} else {
		log.Printf("Order Service: Updating status of order %s from '%s' to '%s'.", req.OrderID, order.Status, req.Status)
		order.Status = req.Status
	}
	ordersDB.Data[req.OrderID] = order

	w.WriteHeader(http.StatusOK)
	response := map[string]string{
		"status":  "success",
		"message": fmt.Sprintf("Order %s status updated to %s", req.OrderID, req.Status),
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Order Service: Error encoding confirm_order response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
