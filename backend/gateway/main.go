package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/StitchMl/saga-demo/common/types"
)

func main() {
	requiredEnv := []string{
		"CHOREOGRAPHER_ORDER_SERVICE_URL", "AUTH_SERVICE_URL",
		"ORCHESTRATOR_MAIN_SERVICE_URL", "GATEWAY_PORT",
	}
	env := make(map[string]string, len(requiredEnv))
	for _, key := range requiredEnv {
		val := os.Getenv(key)
		if val == "" {
			log.Fatalf("[Gateway] Missing env: %s", key)
		}
		env[key] = val
	}
	log.Printf("[Gateway] HTTP server on :%s", env["GATEWAY_PORT"])

	http.HandleFunc("/choreographed_order",
		authenticateRequest(env["AUTH_SERVICE_URL"], createOrderHandler(env["CHOREOGRAPHER_ORDER_SERVICE_URL"])))
	http.HandleFunc("/orchestrated_order",
		authenticateRequest(env["AUTH_SERVICE_URL"], createOrderHandler(env["ORCHESTRATOR_MAIN_SERVICE_URL"])))
	http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SAGA API Gateway attivo!"))
	})
	log.Fatal(http.ListenAndServe(":"+env["GATEWAY_PORT"], nil))
}

// authenticateRequest is middleware that queries an external authentication service.
func authenticateRequest(authServiceURL string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		customerID := r.Header.Get("X-Customer-ID")
		if customerID == "" {
			http.Error(w, "Unauthorized: X-Customer-ID missing", http.StatusUnauthorized)
			return
		}

		validatedID, err := callAuthService(authServiceURL, customerID)
		if err != nil {
			http.Error(w, "Authentication failed", http.StatusUnauthorized)
			return
		}

		var req events.OrderCreatedPayload
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		req.CustomerID = validatedID

		body, _ := json.Marshal(req)
		r.Body = io.NopCloser(bytes.NewBuffer(body))
		r.ContentLength = int64(len(body))
		r.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))

		next.ServeHTTP(w, r)
	}
}

// callAuthService simulates an HTTP call to an external authentication service.
func callAuthService(authServiceURL, customerID string) (string, error) {
	req := map[string]string{"customer_id": customerID}
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("payload error: %w", err)
	}

	resp, err := http.Post(authServiceURL+"/validate", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("auth service error: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("auth service status %d: %s", resp.StatusCode, b)
	}

	var res struct {
		CustomerID string `json:"customer_id"`
		Valid      bool   `json:"valid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", fmt.Errorf("decode error: %w", err)
	}
	if !res.Valid || res.CustomerID == "" {
		return "", fmt.Errorf("invalid credentials")
	}
	return res.CustomerID, nil
}

// createOrderHandler forwards the request to the Order service.
func createOrderHandler(targetServiceURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		req, err := decodeAndValidateOrderRequest(w, r)
		if err != nil {
			return
		}

		statusCode, responseBody, err := sendRequestToTargetService(targetServiceURL, prepareOrderServicePayload(req))
		if err != nil {
			handleTargetServiceError(w, err, statusCode, responseBody)
			return
		}

		if _, err := w.Write(responseBody); err != nil {
			log.Printf("[Gateway] Client response write error: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		} else {
			w.WriteHeader(statusCode)
		}
	}
}

// decodeAndValidateOrderRequest handles the decoding and initial validation of the request.
func decodeAndValidateOrderRequest(w http.ResponseWriter, r *http.Request) (events.OrderCreatedPayload, error) {
	var req events.OrderCreatedPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Richiesta non valida", http.StatusBadRequest)
		return req, err
	}
	if req.CustomerID == "" || len(req.Items) == 0 {
		http.Error(w, "CustomerID e almeno un item sono obbligatori", http.StatusBadRequest)
		return req, fmt.Errorf("dati mancanti")
	}
	for _, item := range req.Items {
		if item.ProductID == "" || item.Quantity <= 0 {
			http.Error(w, "Ogni item deve avere ProductID e Quantity > 0", http.StatusBadRequest)
			return req, fmt.Errorf("item non valido")
		}
	}
	return req, nil
}

// prepareOrderServicePayload constructs the payload to be sent to the target service.
func prepareOrderServicePayload(req events.OrderCreatedPayload) interface{} {
	items := make([]events.OrderItem, 0, len(req.Items))
	for _, item := range req.Items {
		if item.ProductID != "" && item.Quantity > 0 {
			items = append(items, events.OrderItem{
				ProductID: item.ProductID,
				Quantity:  item.Quantity,
				Price:     0.0,
			})
		}
	}
	return events.Order{CustomerID: req.CustomerID, Items: items}
}

// sendRequestToTargetService serializes and sends the request to the target service.
func sendRequestToTargetService(url string, payload interface{}) (int, []byte, error) {
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return http.StatusInternalServerError, nil, err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return 0, nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return http.StatusInternalServerError, nil, err
	}

	return resp.StatusCode, responseBody, nil
}

// handleTargetServiceError handles the communication error with the target service.
func handleTargetServiceError(w http.ResponseWriter, err error, statusCode int, responseBody []byte) {
	log.Printf("[Gateway] Target service error: %v", err)
	if statusCode == 0 {
		http.Error(w, "Destination service connection error", http.StatusBadGateway)
		return
	}
	http.Error(w, string(responseBody), statusCode)
}
