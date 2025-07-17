package inventorydb

import (
	"log"
	"sync"

	"github.com/StitchMl/saga-demo/common/types"
)

// DB contains all the in-memory databases for the various domains
var DB = struct {
	Orders struct {
		sync.RWMutex
		Data map[string]events.Order
	}
	Inventory struct {
		sync.RWMutex
		Data map[string]int
	}
	Prices struct {
		sync.RWMutex
		Data map[string]float64
	}
	Users struct {
		sync.RWMutex
		Data []events.User
	}
}{
	Orders: struct {
		sync.RWMutex
		Data map[string]events.Order
	}{Data: make(map[string]events.Order)},
	Inventory: struct {
		sync.RWMutex
		Data map[string]int
	}{Data: make(map[string]int)},
	Prices: struct {
		sync.RWMutex
		Data map[string]float64
	}{Data: make(map[string]float64)},
	Users: struct {
		sync.RWMutex
		Data []events.User
	}{Data: make([]events.User, 0)},
}

// InitDB initializes the inventory database and price database.
// The service will call this function explicitly.
func InitDB() {
	DB.Orders.Lock()
	DB.Inventory.Lock()
	DB.Prices.Lock()
	DB.Users.Lock()
	defer DB.Orders.Unlock()
	defer DB.Inventory.Unlock()
	defer DB.Prices.Unlock()
	defer DB.Users.Unlock()

	// Inizializzazione degli ordini (vuoto all'inizio)
	DB.Orders.Data = make(map[string]events.Order)

	// Inizializzazione dell'inventario (come prima)
	DB.Inventory.Data = make(map[string]int)
	DB.Inventory.Data["product-A"] = 150
	DB.Inventory.Data["product-B"] = 80
	DB.Inventory.Data["product-C"] = 200
	DB.Inventory.Data["product-D"] = 50
	DB.Inventory.Data["product-E"] = 120

	// Inizializzazione dei prezzi (come prima)
	DB.Prices.Data = make(map[string]float64)
	DB.Prices.Data["product-A"] = 10.50
	DB.Prices.Data["product-B"] = 25.00
	DB.Prices.Data["product-C"] = 5.75
	DB.Prices.Data["product-D"] = 150.00
	DB.Prices.Data["product-E"] = 30.20

	// Initialization of users
	DB.Users.Data = []events.User{
		{
			ID:           "user-1",
			Name:         "Mario Rossi",
			Email:        "mario.rossi@example.com",
			Username:     "mario.rossi",
			PasswordHash: func() string { h, _ := events.HashPassword("password1"); return h }(),
		},
		{
			ID:           "user-2",
			Name:         "Luca Bianchi",
			Email:        "luca.bianchi@example.com",
			Username:     "luca.bianchi",
			PasswordHash: func() string { h, _ := events.HashPassword("password2"); return h }(),
		},
	}

	log.Println("[DataStore] Global data store initialized.")
	log.Println("[DataStore] Initial Inventory:", DB.Inventory.Data)
	log.Println("[DataStore] Initial Prices:", DB.Prices.Data)
}

// GetProductPrice retrieves the price of a product.
func GetProductPrice(productID string) (float64, bool) {
	DB.Prices.RLock()
	defer DB.Prices.RUnlock()
	price, ok := DB.Prices.Data[productID]
	return price, ok
}
