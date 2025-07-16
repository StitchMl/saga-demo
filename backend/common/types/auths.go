package events

// AuthRequest simulates the payload that the gateway would send to the Auth Service.
type AuthRequest struct {
	CustomerID string `json:"customer_id"`
}

// AuthResponse simulates the response that the Auth Service would give to the gateway.
type AuthResponse struct {
	CustomerID string `json:"customer_id"`
	Valid      bool   `json:"valid"`
	Message    string `json:"message,omitempty"`
}

// User Represents a registered user.
type User struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password"`
}
