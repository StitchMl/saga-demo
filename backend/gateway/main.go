package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	ctJSON                  = "application/json"
	ctHdr                   = "Content-Type"
	orderServiceUnreachable = "order service unreachable"
	methodNotAllowed        = "method not allowed"
	validateURL             = "/validate"
)

var gatewayNS = uuid.New()

// mustGet retrieves an environment variable and panics if it is not set.
func mustGet(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("missing env: %s", key)
	}
	return v
}

var (
	chInv = mustGet("CHOREOGRAPHER_INVENTORY_BASE_URL")
	orInv = mustGet("ORCHESTRATOR_INVENTORY_BASE_URL")

	chAuth = mustGet("CHOREOGRAPHER_AUTH_BASE_URL")
	orAuth = mustGet("ORCHESTRATOR_AUTH_BASE_URL")

	chOrder = mustGet("CHOREOGRAPHER_ORDER_BASE_URL")
	orOrder = mustGet("ORCHESTRATOR_ORDER_BASE_URL")

	orchestrator = mustGet("ORCHESTRATOR_SERVICE_URL")
)

// withCORS adds CORS headers to the response and handles preflight requests.
func withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type,X-Customer-ID,X-Auth-NS")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

// Helper: gets customer id from header or query
func customerIDFrom(r *http.Request) string {
	if v := r.Header.Get("X-Customer-ID"); v != "" {
		return v
	}
	return r.URL.Query().Get("customer_id")
}

// Helper: chooses auth URL based on flow
func authURLForFlow(flow string) string {
	if flow == "orchestrated" {
		return orAuth + validateURL
	}
	return chAuth + validateURL
}

// Helper: returns ns from the header/query or gateway fallback
func nsFrom(r *http.Request) string {
	if v := r.Header.Get("X-Auth-NS"); v != "" {
		return v
	}
	if v := r.URL.Query().Get("ns"); v != "" {
		return v
	}
	return gatewayNS.String()
}

// authenticate checks for the X-Customer-ID header and validates it against the auth service.
func authenticate(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cid := customerIDFrom(r)
		if cid == "" {
			http.Error(w, "missing X-Customer-ID", http.StatusUnauthorized)
			return
		}

		authURL := authURLForFlow(r.URL.Query().Get("flow"))
		ns := nsFrom(r)

		body, _ := json.Marshal(map[string]string{
			"customer_id": cid,
			"ns":          ns,
		})

		resp, err := http.Post(authURL, ctJSON, bytes.NewReader(body))
		if err != nil {
			http.Error(w, "auth service unreachable", http.StatusBadGateway)
			return
		}
		defer func() {
			_ = resp.Body.Close()
		}()

		if resp.StatusCode != http.StatusOK {
			http.Error(w, "authentication failed", http.StatusUnauthorized)
			return
		}

		var authResp struct {
			Valid bool `json:"valid"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil || !authResp.Valid {
			http.Error(w, "authentication failed", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

// createOrderHandler handles order creation requests and proxies them to the appropriate service.
func createOrderHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, methodNotAllowed, http.StatusMethodNotAllowed)
		return
	}

	flow := r.URL.Query().Get("flow")
	baseURL := chOrder
	if flow == "orchestrated" {
		baseURL = orchestrator
	}

	client := &http.Client{Timeout: 15 * time.Second}
	url := baseURL + "/create_order"
	// The customer ID is in the header, not the body.
	bodyBytes, _ := io.ReadAll(r.Body)
	_ = r.Body.Close() // Close the original body

	var orderData map[string]interface{}
	_ = json.Unmarshal(bodyBytes, &orderData)
	orderData["customer_id"] = r.Header.Get("X-Customer-ID")
	newBody, _ := json.Marshal(orderData)

	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(newBody))
	req.Header.Set(ctHdr, ctJSON)

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, orderServiceUnreachable, http.StatusBadGateway)
		return
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	w.Header().Set(ctHdr, ctJSON)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// ordersHandler dispatches requests to /orders based on the HTTP method.
func ordersHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		ordersListProxy(w, r)
	case http.MethodPost:
		createOrderHandler(w, r)
	default:
		http.Error(w, methodNotAllowed, http.StatusMethodNotAllowed)
	}
}

// catalogProxy retrieves the catalog from the appropriate inventory service based on the flow type.
func catalogProxy(w http.ResponseWriter, r *http.Request) {
	base := chInv
	if r.URL.Query().Get("flow") == "orchestrated" {
		base = orInv
	}
	resp, err := http.Get(base + "/catalog")
	if err != nil || resp.StatusCode != http.StatusOK {
		http.Error(w, "inventory unreachable", http.StatusBadGateway)
		return
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	w.Header().Set(ctHdr, ctJSON)
	_, _ = io.Copy(w, resp.Body)
}

// ordersListProxy retrieves the list of orders for a customer from the appropriate order service.
func ordersListProxy(w http.ResponseWriter, r *http.Request) {
	cid := r.URL.Query().Get("customer_id")
	if cid == "" {
		http.Error(w, "customer_id required", http.StatusBadRequest)
		return
	}
	base := chOrder
	if r.URL.Query().Get("flow") == "orchestrated" {
		base = orOrder
	}
	url := fmt.Sprintf("%s/orders?customer_id=%s", base, cid)
	resp, err := http.Get(url)
	if err != nil || resp.StatusCode != http.StatusOK {
		http.Error(w, orderServiceUnreachable, http.StatusBadGateway)
		return
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	w.Header().Set(ctHdr, ctJSON)
	_, _ = io.Copy(w, resp.Body)
}

// orderStatusProxy retrieves the status of a specific order by ID from the appropriate order service.
func orderStatusProxy(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	flow := r.URL.Query().Get("flow")
	base := chOrder
	if flow == "orchestrated" {
		base = orOrder
	}
	resp, err := http.Get(base + "/orders/" + id)
	if err != nil {
		http.Error(w, orderServiceUnreachable, http.StatusBadGateway)
		return
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	w.Header().Set(ctHdr, ctJSON)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// authProxy handles authentication requests and proxies them to the appropriate auth service.
func authProxy(w http.ResponseWriter, r *http.Request) {
	flow := r.URL.Query().Get("flow")
	base := chAuth
	if flow == "orchestrated" {
		base = orAuth
	}
	url := base + r.URL.Path

	// Read the original body
	var payload map[string]interface{}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&payload)
	}
	if payload == nil {
		payload = map[string]interface{}{}
	}

	ns := r.Header.Get("X-Auth-NS")
	if ns == "" {
		ns = r.URL.Query().Get("ns")
	}
	if ns == "" {
		ns = gatewayNS.String()
	}

	// Inject ns ALWAYS for auth routes (/register, /login, /validate)
	payload["ns"] = ns
	buf, _ := json.Marshal(payload)

	req, _ := http.NewRequest(r.Method, url, bytes.NewReader(buf))
	req.Header.Set(ctHdr, ctJSON)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "auth unreachable", http.StatusBadGateway)
		return
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	w.Header().Set(ctHdr, ctJSON)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func main() {
	port := mustGet("GATEWAY_PORT")

	http.HandleFunc("/orders", withCORS(authenticate(ordersHandler))) // Use the new dispatcher
	http.HandleFunc("/orders/", withCORS(authenticate(orderStatusProxy)))

	http.HandleFunc("/catalog", withCORS(catalogProxy))

	http.HandleFunc("/register", withCORS(authProxy))
	http.HandleFunc("/login", withCORS(authProxy))
	http.HandleFunc(validateURL, withCORS(authProxy))

	http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Gateway OK"))
	})

	log.Printf("[Gateway] listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
