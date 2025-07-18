package inventorydb

import (
	"log"
	"sync"

	events "github.com/StitchMl/saga-demo/common/types"
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
}{}

// InitDB initializes the inventory database and price database.
func InitDB() {
	DB.Orders.Lock()
	DB.Inventory.Lock()
	DB.Prices.Lock()
	DB.Users.Lock()
	defer DB.Orders.Unlock()
	defer DB.Inventory.Unlock()
	defer DB.Prices.Unlock()
	defer DB.Users.Unlock()

	DB.Orders.Data = make(map[string]events.Order)
	DB.Inventory.Data = map[string]int{
		"prod-1": 100,
		"prod-2": 75,
		"prod-3": 42,
	}
	DB.Prices.Data = map[string]float64{
		"prod-1": 19.90,
		"prod-2": 34.50,
		"prod-3": 12.00,
	}
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

// GetOrder returns an order by ID if it exists.
func GetOrder(orderID string) (events.Order, bool) {
	DB.Orders.RLock()
	defer DB.Orders.RUnlock()
	o, ok := DB.Orders.Data[orderID]
	return o, ok
}
