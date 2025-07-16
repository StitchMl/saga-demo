package events

import "time"

// Transaction represents a financial transaction
type Transaction struct {
	OrderID     string
	CustomerID  string
	Amount      float64
	Status      string
	Timestamp   time.Time
	ErrorReason string
	Attempts    int
}
