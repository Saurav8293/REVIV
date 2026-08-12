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
		last := lastHeartbeat
		mu.Unlock()

		timeSinceLastHeartbeat := time.Since(last)

		if timeSinceLastHeartbeat > 30*time.Second {
			fmt.Println("HEARTBEAT LOST!")
		}

	}
}
