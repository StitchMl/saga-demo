package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"
)

// Event rappresenta il payload pubblicato
type Event struct {
	Type string          `json:"Type"`
	Data json.RawMessage `json:"Data"`
}

var (
	subscribers = make(map[string][]string)
	mu          sync.Mutex
)

// writeCORS aggiunge gli header CORS
func writeCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func publish(w http.ResponseWriter, r *http.Request) {
	writeCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "cannot read body", http.StatusBadRequest)
		log.Printf("publish read error: %v", err)
		return
	}
	var e Event
	if err := json.Unmarshal(body, &e); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		log.Printf("publish decode error: %v", err)
		return
	}

	mu.Lock()
	sinks := append([]string{}, subscribers[e.Type]...)
	mu.Unlock()

	for _, url := range sinks {
		go func(u string) {
			resp, err := http.Post(u, "application/json", bytes.NewReader(body))
			if err != nil {
				log.Printf("publish: POST %s failed: %v", u, err)
				return
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode >= 300 {
				log.Printf("publish: non-OK %d from %s", resp.StatusCode, u)
			}
		}(url)
	}
	w.WriteHeader(http.StatusOK)
}

func subscribe(w http.ResponseWriter, r *http.Request) {
	writeCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	var req struct {
		Type string `json:"Type"`
		Url  string `json:"Url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		log.Printf("subscribe decode error: %v", err)
		return
	}

	mu.Lock()
	subscribers[req.Type] = append(subscribers[req.Type], req.Url)
	mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

func main() {
	http.HandleFunc("/publish", publish)
	http.HandleFunc("/subscribe", subscribe)
	log.Println("Event Bus listening on :8070")
	log.Fatal(http.ListenAndServe(":8070", nil))
}
