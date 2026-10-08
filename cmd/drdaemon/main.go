package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"REVIV/internal/config"
	"REVIV/internal/heartbeat"
)

var (
	startTime       = time.Now()
	lastWALTime     = time.Now()
	lastWALSequence = "00000001000000000000001C.lz4"
)

func checkPostgresConnection(host string, port int) string {
	timeout := 1 * time.Second
	target := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.DialTimeout("tcp", target, timeout)
	if err != nil {
		return "DOWN"
	}
	_ = conn.Close()
	return "UP"
}

func main() {
	configPath := flag.String("config", "configs/dr.yaml", "Path to dr.yaml configuration")
	flag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Printf("[WARN] Failed to load config from %s: %v. Using defaults.", *configPath, err)
		cfg = config.DefaultConfig()
	}

	fmt.Println("=====================================================")
	fmt.Println("      REVIV: Disaster Recovery Primary Daemon (drdaemon) ")
	fmt.Println("=====================================================")
	fmt.Printf("Node ID               : %s\n", cfg.Primary.NodeID)
	fmt.Printf("Heartbeat Target      : %s\n", cfg.Primary.HeartbeatURL)
	fmt.Printf("Interval              : %ds\n", cfg.Primary.HeartbeatIntervalSecond)
	fmt.Printf("PostgreSQL Target     : %s:%d (DB: %s)\n", cfg.Primary.DBHost, cfg.Primary.DBPort, cfg.Primary.DBName)
	fmt.Printf("S3 WAL-G Prefix       : s3://%s/%s\n", cfg.Backup.S3Bucket, cfg.Backup.WALPrefix)
	fmt.Println("-----------------------------------------------------")

	client := heartbeat.NewClient(cfg.Primary.HeartbeatURL)

	// Local status server on port :8090
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		dbStatus := checkPostgresConnection(cfg.Primary.DBHost, cfg.Primary.DBPort)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"node_id":            cfg.Primary.NodeID,
			"uptime_seconds":     int64(time.Since(startTime).Seconds()),
			"db_status":          dbStatus,
			"last_wal_pushed_at": lastWALTime,
			"s3_bucket":          cfg.Backup.S3Bucket,
		})
	})

	localServer := &http.Server{
		Addr:         ":8090",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("[drdaemon] Local status server listening on http://localhost:8090/status")
		if err := localServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[drdaemon] Local HTTP server error: %v", err)
		}
	}()

	interval := time.Duration(cfg.Primary.HeartbeatIntervalSecond) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	log.Printf("[drdaemon] Starting continuous heartbeat telemetry loop...")

	// Initial heartbeat immediately
	sendHeartbeatPacket(client, cfg)

	for {
		select {
		case <-sigChan:
			log.Println("[drdaemon] Received stop signal, shutting down...")
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = localServer.Shutdown(ctx)
			return

		case <-ticker.C:
			sendHeartbeatPacket(client, cfg)
		}
	}
}

func sendHeartbeatPacket(client *heartbeat.Client, cfg *config.Config) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	dbStatus := checkPostgresConnection(cfg.Primary.DBHost, cfg.Primary.DBPort)

	// In real setup, WAL-G pushes WAL every 30-60s or when segment fills up
	if time.Since(lastWALTime) > 30*time.Second {
		lastWALTime = time.Now()
	}

	payload := heartbeat.HeartbeatPayload{
		NodeID:          cfg.Primary.NodeID,
		Timestamp:       time.Now(),
		DBStatus:        dbStatus,
		LastWALPushedAt: lastWALTime,
		LastWALSequence: lastWALSequence,
		DiskFreePct:     68.4,
		MemoryFreePct:   54.1,
		UptimeSeconds:   int64(time.Since(startTime).Seconds()),
		Version:         "1.0.0",
	}

	err := client.SendHeartbeat(ctx, payload)
	if err != nil {
		log.Printf("[drdaemon] Warning: Heartbeat transmission failed: %v", err)
	} else {
		log.Printf("[drdaemon] Heartbeat dispatched successfully (DB: %s, Lag: %ds)",
			dbStatus, int(time.Since(lastWALTime).Seconds()))
	}
}
