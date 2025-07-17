package events

import (
	"golang.org/x/crypto/bcrypt"
)

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
	ID           string `json:"id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	Username     string `json:"username" binding:"required"`
	PasswordHash string `json:"password" binding:"required"`
}

// HashPassword genera un hash della password.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword verifica se la password corrisponde al suo hash.
func CheckPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
