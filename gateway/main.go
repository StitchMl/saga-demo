package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/StitchMl/saga-demo/common/events"
	"io"
	"log"
	"net/http"
	"os"
)

// OrderRequest is the request a user would send to create an order.
type OrderRequest struct {
	CustomerID string `json:"customer_id"`
	Items      []struct {
		ProductID string `json:"product_id"`
		Quantity  int    `json:"quantity"`
	} `json:"items"`
}

func main() {
	// Retrieves service URLs from environment parameters
	choreographerOrderServiceURL := os.Getenv("CHOREOGRAPHER_ORDER_SERVICE_URL")
	if choreographerOrderServiceURL == "" {
		log.Fatal("[Gateway] Environment variable CHOREOGRAPHER_ORDER_SERVICE_URL not set. Required.")
	}

	orchestratorMainServiceURL := os.Getenv("ORCHESTRATOR_MAIN_SERVICE_URL")
	if orchestratorMainServiceURL == "" {
		log.Fatal("[Gateway] ORCHESTRATOR_MAIN_SERVICE_URL environment variable not set. Required.")
	}

	authServiceURL := os.Getenv("AUTH_SERVICE_URL")
	if authServiceURL == "" {
		log.Fatal("[Gateway] Environment variable AUTH_SERVICE_URL not set. It is mandatory for realistic authentication.")
	}

	gatewayPort := os.Getenv("GATEWAY_PORT")
	if gatewayPort == "" {
		gatewayPort = "8000"
		log.Printf("[Gateway] GATEWAY_PORT variable not set, default usage: %s", gatewayPort)
	}

	log.Println("[Gateway] Starting the HTTP server of the Gateway API on port :" + gatewayPort)

	// The 'authenticateRequest' middleware applies authentication before forwarding the request.
	http.HandleFunc("/choreographed_order", authenticateRequest(authServiceURL, createOrderHandler(choreographerOrderServiceURL)))

	// Again, the middleware 'authenticateRequest' applies authentication.
	http.HandleFunc("/orchestrated_order", authenticateRequest(authServiceURL, createOrderHandler(orchestratorMainServiceURL)))

	// Endpoint di Health Check per il Gateway stesso
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("The SAGA API Gateway is healthy!")); err != nil {
			log.Printf("[Gateway] Error when writing response for /health: %v", err)
		}
	})

	// Start the HTTP server
	log.Fatal(http.ListenAndServe(":"+gatewayPort, nil))
}

// authenticateRequest is middleware that queries an external authentication service.
func authenticateRequest(authServiceURL string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("X-Customer-ID")
		if authHeader == "" {
			http.Error(w, "Unauthorized access: header X-Customer-ID (or token) missing", http.StatusUnauthorized)
			return
		}

		validatedCustomerID, err := callAuthService(authServiceURL, authHeader)
		if err != nil {
			log.Printf("[Gateway] Error while calling authentication service: %v", err)
			http.Error(w, fmt.Sprintf("Authentication failed: %v", err), http.StatusUnauthorized)
			return
		}

		// Reads the original body of the request
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("[Gateway] Error while reading request body: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		// Resets the body for the next handler
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		var req OrderRequest
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			log.Printf("[Gateway] Error when deserializing body to update CustomerID: %v", err)
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		// Overwrites the CustomerID in the request payload with the authenticated one.
		req.CustomerID = validatedCustomerID

		// Recode the updated payload
		updatedBodyBytes, err := json.Marshal(req)
		if err != nil {
			log.Printf("[Gateway] Error when re-serializing body with updated CustomerID: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Replaces the request body with the updated body.
		r.Body = io.NopCloser(bytes.NewBuffer(updatedBodyBytes))
		r.ContentLength = int64(len(updatedBodyBytes))
		r.Header.Set("Content-Length", fmt.Sprintf("%d", len(updatedBodyBytes)))

		log.Printf("[Gateway] Authenticated request for CustomerID: %s. Forwarding.", validatedCustomerID)

		next.ServeHTTP(w, r)
	}
}

// callAuthService simulates an HTTP call to an external authentication service.
func callAuthService(authServiceURL, authCredential string) (string, error) {
	reqPayload := map[string]string{"customer_id": authCredential}
	jsonBody, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("unable to serialize authentication payload: %w", err)
	}

	resp, err := http.Post(authServiceURL+"/validate", "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("authentication service connection error: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("[Gateway] Error while closing response body from authentication service: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("the authentication service returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var authResponse struct {
		CustomerID string `json:"customer_id"`
		Valid      bool   `json:"valid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&authResponse); err != nil {
		return "", fmt.Errorf("error while decoding authentication response: %w", err)
	}

	if !authResponse.Valid || authResponse.CustomerID == "" {
		return "", fmt.Errorf("invalid credentials or Customer ID not provided by the authentication service")
	}

	return authResponse.CustomerID, nil
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

		log.Printf("[Gateway] Order creation request received for Customer %s. Forwarded to: %s", req.CustomerID, targetServiceURL)

		// Prepares the payload for the target service
		orderServicePayload := prepareOrderServicePayload(req)

		// Serialize and forward the request
		statusCode, responseBody, err := sendRequestToTargetService(targetServiceURL, orderServicePayload)
		if err != nil {
			handleTargetServiceError(w, err, statusCode, responseBody)
			return
		}

		// Propagates the response of the target service to the client
		w.WriteHeader(statusCode)
		if _, writeErr := w.Write(responseBody); writeErr != nil {
			log.Printf("[Gateway] Error when writing response to client: %v", writeErr)
		}
	}
}

// decodeAndValidateOrderRequest handles the decoding and initial validation of the request.
func decodeAndValidateOrderRequest(w http.ResponseWriter, r *http.Request) (OrderRequest, error) {
	var req OrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Body della richiesta non valido", http.StatusBadRequest)
		return req, fmt.Errorf("decoding error: %w", err)
	}

	if req.CustomerID == "" {
		http.Error(w, "CustomerID is mandatory in the body of the request", http.StatusBadRequest)
		return req, fmt.Errorf("missing CustomerID")
	}

	if len(req.Items) == 0 {
		http.Error(w, "The order must contain at least one item", http.StatusBadRequest)
		return req, fmt.Errorf("no items in the order")
	}

	for _, item := range req.Items {
		if item.ProductID == "" {
			http.Error(w, "ProductID cannot be empty", http.StatusBadRequest)
			return req, fmt.Errorf("empty ProductID")
		}
		if item.Quantity <= 0 {
			http.Error(w, "The quantity must be greater than zero", http.StatusBadRequest)
			return req, fmt.Errorf("quantity must be positive")
		}
	}
	return req, nil
}

// prepareOrderServicePayload constructs the payload to be sent to the target service.
func prepareOrderServicePayload(req OrderRequest) interface{} {
	var order events.Order
	order.CustomerID = req.CustomerID
	order.Items = make([]events.OrderItem, len(req.Items))
	for i, item := range req.Items {
		order.Items[i].ProductID = item.ProductID
		order.Items[i].Quantity = item.Quantity
		order.Items[i].Price = 0.0
	}
	// OrderID and Status will be generated and managed by the receiving service.
	return order
}

// sendRequestToTargetService serializes and sends the request to the target service.
func sendRequestToTargetService(url string, payload interface{}) (int, []byte, error) {
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return http.StatusInternalServerError, nil, fmt.Errorf("unable to serialize payload: %w", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return 0, nil, fmt.Errorf("error while sending HTTP request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("[Gateway] Error while closing response body from target service (%s): %v", url, closeErr)
		}
	}()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return http.StatusInternalServerError, nil, fmt.Errorf("error while reading the response: %w", err)
	}

	return resp.StatusCode, responseBody, nil
}

// handleTargetServiceError handles the communication error with the target service.
func handleTargetServiceError(w http.ResponseWriter, err error, statusCode int, responseBody []byte) {
	log.Printf("[Gateway] Error while sending or receiving from destination service: %v", err)
	if statusCode == 0 {
		http.Error(w, "Error connecting to the destination service", http.StatusBadGateway)
	} else {
		w.WriteHeader(statusCode)
		if _, writeErr := w.Write(responseBody); writeErr != nil {
			log.Printf("[Gateway] Error when writing response to client: %v", writeErr)
		}
	}
}
