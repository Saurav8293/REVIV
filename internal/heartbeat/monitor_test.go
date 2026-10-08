package heartbeat

import (
	"sync"
	"testing"
	"time"

	"REVIV/internal/config"
)

func TestHeartbeatThresholdAndRecovery(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Heartbeat.CheckInterval = 10 * time.Millisecond
	cfg.Primary.HeartbeatIntervalSecond = 1
	cfg.Heartbeat.SuspectThresholdMisses = 2
	cfg.Heartbeat.DownThresholdMisses = 4

	monitor := NewMonitor(cfg)

	// Verify initial healthy state
	st := monitor.GetStatus()
	if st.Status != StatusHealthy {
		t.Fatalf("expected initial status HEALTHY, got %s", st.Status)
	}

	// Record initial heartbeat
	monitor.RecordHeartbeat(HeartbeatPayload{
		NodeID:          "primary-vps-01",
		Timestamp:       time.Now(),
		DBStatus:        "UP",
		LastWALPushedAt: time.Now(),
	})

	// Simulate outage by setting lastHeartbeat into the past
	monitor.mu.Lock()
	monitor.lastHeartbeat = time.Now().Add(-5 * time.Second)
	monitor.mu.Unlock()

	monitor.evaluateHeartbeatRecency()

	st = monitor.GetStatus()
	if st.Status != StatusDown {
		t.Fatalf("expected status DOWN after threshold breach, got %s", st.Status)
	}
	if !st.OperatorConfirmationRequired {
		t.Fatalf("expected OperatorConfirmationRequired to be true")
	}
	if st.OperatorConfirmationToken == "" {
		t.Fatalf("expected non-empty OperatorConfirmationToken")
	}

	// Verify confirmation with valid token
	confirmResp, err := monitor.ConfirmFailover(OperatorConfirmRequest{
		Token:    st.OperatorConfirmationToken,
		Operator: "SecOps-Admin",
		Reason:   "Confirmed VPS unresponsive",
	})
	if err != nil || !confirmResp.Success {
		t.Fatalf("expected confirmation to succeed with valid token, got err: %v", err)
	}

	st = monitor.GetStatus()
	if !st.ConfirmedByOperator {
		t.Fatalf("expected ConfirmedByOperator to be true")
	}

	// Verify recovery resets state
	monitor.RecordHeartbeat(HeartbeatPayload{
		NodeID:          "primary-vps-01",
		Timestamp:       time.Now(),
		DBStatus:        "UP",
		LastWALPushedAt: time.Now(),
	})

	st = monitor.GetStatus()
	if st.Status != StatusHealthy {
		t.Fatalf("expected recovered status HEALTHY, got %s", st.Status)
	}
	if st.OperatorConfirmationRequired {
		t.Fatalf("expected OperatorConfirmationRequired to be false after recovery")
	}
}

func TestConcurrentHeartbeats(t *testing.T) {
	cfg := config.DefaultConfig()
	monitor := NewMonitor(cfg)

	var wg sync.WaitGroup
	workers := 20
	iterations := 50

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				monitor.RecordHeartbeat(HeartbeatPayload{
					NodeID:          "primary-vps-01",
					Timestamp:       time.Now(),
					DBStatus:        "UP",
					LastWALPushedAt: time.Now(),
					DiskFreePct:     70.0,
					MemoryFreePct:   60.0,
				})
				_ = monitor.GetStatus()
			}
		}(i)
	}

	wg.Wait()

	st := monitor.GetStatus()
	if st.Status != StatusHealthy {
		t.Fatalf("expected status HEALTHY after concurrent pings, got %s", st.Status)
	}
}
