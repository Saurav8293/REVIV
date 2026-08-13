package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

var (
	lastHeartbeat time.Time
	heartbeatLost bool
	mu            sync.Mutex
)

func main() {

	lastHeartbeat = time.Now()
	mux := http.NewServeMux()

	mux.HandleFunc("/health", checkHealth)

	server := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go monitorHeartbeat()

	fmt.Println("Server is running on port: 8080")
	log.Fatal(server.ListenAndServe())

}

func checkHealth(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	lastHeartbeat = time.Now()

	if heartbeatLost {
		fmt.Println("HEARTBEAT RECOVERED!")
		heartbeatLost = false
	}

	mu.Unlock()

	fmt.Println("HEARTBEAT RECEIVED")

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Server is up!")
}

func monitorHeartbeat() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		<-ticker.C

		mu.Lock()

		timeSinceLastHeartbeat := time.Since(lastHeartbeat)

		if timeSinceLastHeartbeat > 30*time.Second {
			if !heartbeatLost {
				fmt.Println("HEARTBEAT LOST!")
				heartbeatLost = true
			}
		}
		mu.Unlock()
	}
}
