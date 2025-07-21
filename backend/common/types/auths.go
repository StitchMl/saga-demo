package events

import (
	"golang.org/x/crypto/bcrypt"
)

// User Represents a registered user.
// During registration, the Password field will contain the plain text password.
// When stored or retrieved, the PasswordHash field will contain the hash.
type User struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	Username     string `json:"username" binding:"required"`
	Password     string `json:"password,omitempty" binding:"required"`
	PasswordHash string `json:"-"` // Excluded from JSON responses
}

// AuthRequest represents the payload for a login request.
type AuthRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// AuthResponse represents the payload for a successful login response.
type AuthResponse struct {
	CustomerID string `json:"customer_id"`
	Valid      bool   `json:"valid"`
}

// HashPassword generates a hash of the password.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword checks whether the password matches its hash.
func CheckPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
