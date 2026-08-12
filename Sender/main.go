package main

import (
	"fmt"
	"net/http"
	"time"
)

func main() {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		resp, err := client.Get("http://localhost:8080/health")
		if err != nil {
			fmt.Printf("health check failed: %v\n", err)
			return
		}

		if resp.StatusCode == http.StatusOK {
			fmt.Println("Server is reachable!")
		}
		resp.Body.Close()

		<-ticker.C
	}

}
