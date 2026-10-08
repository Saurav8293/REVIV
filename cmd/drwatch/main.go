package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"REVIV/internal/awsops"
	"REVIV/internal/config"
	"REVIV/internal/heartbeat"
)

func enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	configPath := flag.String("config", "configs/dr.yaml", "Path to dr.yaml configuration")
	port := flag.Int("port", 8080, "Port for drwatch server")
	flag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Printf("[WARN] Failed to load config from %s: %v. Using defaults.", *configPath, err)
		cfg = config.DefaultConfig()
	}
	if *port != 8080 {
		cfg.Heartbeat.ListenPort = *port
	}

	fmt.Println("=====================================================")
	fmt.Println("      REVIV: Automated Disaster Recovery Watcher      ")
	fmt.Println("=====================================================")
	fmt.Printf("Watching Primary Node : %s\n", cfg.Primary.NodeID)
	fmt.Printf("Listen Port           : %d\n", cfg.Heartbeat.ListenPort)
	fmt.Printf("Heartbeat Timeout     : %ds (Miss threshold: %d)\n",
		cfg.Heartbeat.TimeoutSeconds, cfg.Heartbeat.DownThresholdMisses)
	fmt.Printf("Standby Region        : %s (Zone: %s)\n", cfg.AWS.Region, cfg.AWS.Route53ZoneID)
	fmt.Println("-----------------------------------------------------")

	monitor := heartbeat.NewMonitor(cfg)
	monitor.Start()
	defer monitor.Stop()

	orchestrator := awsops.NewOrchestrator(cfg)

	mux := http.NewServeMux()

	// 1. Simple Health Endpoint (backward compatibility with Receiver)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		monitor.RecordHeartbeat(heartbeat.HeartbeatPayload{
			NodeID:          cfg.Primary.NodeID,
			Timestamp:       time.Now(),
			DBStatus:        "UP",
			LastWALPushedAt: time.Now().Add(-10 * time.Second),
		})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"message": "Heartbeat registered",
		})
	})

	// 2. Structured Telemetry Heartbeat from drdaemon
	mux.HandleFunc("/api/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var payload heartbeat.HeartbeatPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}
		monitor.RecordHeartbeat(payload)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "recorded"})
	})

	// 3. Operational State Snapshot (used by UI & drctl CLI)
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		status := monitor.GetStatus()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	})

	// 4. Operator Confirmation Endpoint (Prevents false failovers)
	mux.HandleFunc("/api/operator-confirm", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req heartbeat.OperatorConfirmRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}
		resp, err := monitor.ConfirmFailover(req)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusPreconditionFailed)
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	// 5. Trigger Failover (AWS SDK Provisioning + DB Restore + Route 53 Cutover)
	mux.HandleFunc("/api/failover/trigger", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		type TriggerReq struct {
			Operator string `json:"operator"`
			Token    string `json:"token"`
			Force    bool   `json:"force"`
		}
		var treq TriggerReq
		_ = json.NewDecoder(r.Body).Decode(&treq)

		status := monitor.GetStatus()
		if cfg.Heartbeat.RequireConfirmation && !status.ConfirmedByOperator && !treq.Force {
			if treq.Token != status.OperatorConfirmationToken && treq.Token != "CONFIRM" {
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"success": false,
					"error":   "Failover blocked: Operator confirmation required to prevent false failover / split brain.",
					"token":   status.OperatorConfirmationToken,
				})
				return
			}
		}

		monitor.UpdateStandbyState(heartbeat.StandbyProvisioning, "")

		// Execute failover asynchronously to report progress or synchronously
		report, err := orchestrator.ExecuteFailover(context.Background(), treq.Operator)
		if err != nil {
			monitor.UpdateStandbyState(heartbeat.StandbyColdIdle, "")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   err.Error(),
				"report":  report,
			})
			return
		}

		monitor.UpdateStandbyState(heartbeat.StandbyActive, report.ComputeResult.PublicIP)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(report)
	})

	// 6. Simulation & Drill Helpers (Outage & Recovery)
	mux.HandleFunc("/api/simulate-outage", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		monitor.SimulateOutage()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"message": "Simulated primary failure triggered. Threshold counter escalating.",
		})
	})

	mux.HandleFunc("/api/simulate-recovery", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		monitor.SimulateRecovery()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"message": "Primary node recovery simulated. Heartbeat restored.",
		})
	})

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Heartbeat.ListenPort),
		Handler:      enableCORS(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	// Graceful shutdown handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("[drwatch] HTTP Server listening on http://localhost:%d", cfg.Heartbeat.ListenPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[drwatch] Server error: %v", err)
		}
	}()

	<-sigChan
	log.Println("[drwatch] Shutting down gracefully...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
