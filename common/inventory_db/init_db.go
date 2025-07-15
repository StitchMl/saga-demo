package inventorydb

import (
	"log"
	"sync"
)

// DB is the in-memory database for inventory. It must be exported.
var DB = struct {
	sync.RWMutex
	Data map[string]int // Map ProductID to available quantity
}{Data: make(map[string]int)}

// PriceDB is the in-memory database for product prices.
var PriceDB = struct {
	sync.RWMutex
	Data map[string]float64 // Map ProductID to price
}{Data: make(map[string]float64)}

// InitDB initializes the inventory database and price database.
// The service will call this function explicitly.
func InitDB() {
	DB.Lock()
	PriceDB.Lock()
	defer DB.Unlock()
	defer PriceDB.Unlock()

	DB.Data = make(map[string]int)
	DB.Data["product-A"] = 150
	DB.Data["product-B"] = 80
	DB.Data["product-C"] = 200
	DB.Data["product-D"] = 50
	DB.Data["product-E"] = 120

	PriceDB.Data = make(map[string]float64)
	PriceDB.Data["product-A"] = 10.50
	PriceDB.Data["product-B"] = 25.00
	PriceDB.Data["product-C"] = 5.75
	PriceDB.Data["product-D"] = 150.00
	PriceDB.Data["product-E"] = 30.20

	log.Println("[InventoryDB] Inventory database initialized:", DB.Data)
	log.Println("[InventoryDB] Price database initialized:", PriceDB.Data)
}

// GetProductPrice retrieves the price of a product.
func GetProductPrice(productID string) (float64, bool) {
	PriceDB.RLock()
	defer PriceDB.RUnlock()
	price, ok := PriceDB.Data[productID]
	return price, ok
}
