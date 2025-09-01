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
	Products struct {
		sync.RWMutex
		Data map[string]events.Product
	}
	Users struct {
		sync.RWMutex
		Data []events.User
	}
}{}

// InitDB initializes the inventory database and price database.
func InitDB() {
	DB.Orders.Data = make(map[string]events.Order)
	DB.Products.Data = map[string]events.Product{
		"laptop-pro":          {ID: "laptop-pro", Name: "Laptop Pro", Description: "A powerful laptop for professionals.", Price: 1299.99, Available: 100, ImageURL: "https://m.media-amazon.com/images/I/61UcV2bDnoL._AC_SL1500_.jpg"},
		"mouse-wireless":      {ID: "mouse-wireless", Name: "Mouse Wireless", Description: "Ergonomic and precise mouse.", Price: 49.50, Available: 50, ImageURL: "https://m.media-amazon.com/images/I/711bP+FjSQL._AC_SL1500_.jpg"},
		"mechanical-keyboard": {ID: "mechanical-keyboard", Name: "Keyboard Mechanical", Description: "Keyboard with mechanical switches for gaming.", Price: 120.00, Available: 200, ImageURL: "https://m.media-amazon.com/images/I/71kq6u7NA4L._AC_SL1500_.jpg"},
	}

	u1hash, _ := events.HashPassword("pass1")
	u2hash, _ := events.HashPassword("pass2")
	DB.Users.Data = []events.User{
		{
			ID:           "user1",
			Name:         "Mario Rossi",
			Email:        "mario.rossi@example.com",
			Username:     "user1",
			PasswordHash: u1hash,
		},
		{
			ID:           "user2",
			Name:         "Luca Bianchi",
			Email:        "luca.bianchi@example.com",
			Username:     "user2",
			PasswordHash: u2hash,
		},
	}
	log.Println("[DataStore] In-memory database initialized with sample data.")
}

// GetProductPrice retrieves the price of a product.
func GetProductPrice(productID string) (float64, bool) {
	DB.Products.RLock()
	defer DB.Products.RUnlock()
	product, ok := DB.Products.Data[productID]
	if !ok {
		return 0, false
	}
	return product.Price, true
}

// GetOrder returns an order by ID if it exists.
func GetOrder(orderID string) (events.Order, bool) {
	DB.Orders.RLock()
	defer DB.Orders.RUnlock()
	order, ok := DB.Orders.Data[orderID]
	return order, ok
}
