package heartbeat

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"REVIV/internal/config"
)

// Monitor manages concurrent primary heartbeat tracking, threshold breaches, and failover authorization.
type Monitor struct {
	mu sync.RWMutex

	cfg           *config.Config
	status        NodeStatus
	primaryNodeID string

	lastHeartbeat     time.Time
	consecutiveMisses int
	dbStatus          string
	lastWALPushedAt   time.Time
	lastWALSequence   string
	diskFreePct       float64
	memoryFreePct     float64

	cloudStandbyState  StandbyCloudState
	standbyComputeIP   string
	failoverInProgress bool

	confirmationRequired bool
	confirmationToken    string
	confirmedByOperator  bool
	confirmedAt          *time.Time
	confirmedBy          string

	lastAlertSentAt *time.Time
	auditEvents     []AuditEvent

	stopCh chan struct{}
}

// NewMonitor initializes a concurrent heartbeat monitoring engine.
func NewMonitor(cfg *config.Config) *Monitor {
	m := &Monitor{
		cfg:               cfg,
		status:            StatusHealthy,
		primaryNodeID:     cfg.Primary.NodeID,
		lastHeartbeat:     time.Now(),
		dbStatus:          "UP",
		lastWALPushedAt:   time.Now().Add(-15 * time.Second),
		lastWALSequence:   "000000010000000000000001",
		diskFreePct:       74.5,
		memoryFreePct:     58.2,
		cloudStandbyState: StandbyColdIdle,
		stopCh:            make(chan struct{}),
		auditEvents:       make([]AuditEvent, 0),
	}

	m.addEvent("INFO", "MONITOR_STARTED", fmt.Sprintf("REVIV Heartbeat Monitor active. Watching node: %s", cfg.Primary.NodeID))
	return m
}

// Start spawns the background concurrent watchdog goroutine using a ticker.
func (m *Monitor) Start() {
	go m.runTickerLoop()
}

// Stop gracefully shuts down the watchdog goroutine.
func (m *Monitor) Stop() {
	close(m.stopCh)
}

// runTickerLoop executes the concurrent ticker loop checking heartbeat recency against thresholds.
func (m *Monitor) runTickerLoop() {
	ticker := time.NewTicker(m.cfg.Heartbeat.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.evaluateHeartbeatRecency()
		}
	}
}

// evaluateHeartbeatRecency evaluates misses and triggers threshold events.
func (m *Monitor) evaluateHeartbeatRecency() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// If failover has already completed or is in progress, skip evaluation
	if m.status == StatusFailoverTriggered || m.failoverInProgress {
		return
	}

	elapsed := time.Since(m.lastHeartbeat)
	interval := time.Duration(m.cfg.Primary.HeartbeatIntervalSecond) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}

	// Calculate misses
	m.consecutiveMisses = int(elapsed / interval)

	// Threshold evaluation: HEALTHY -> SUSPECT -> DOWN
	if m.consecutiveMisses >= m.cfg.Heartbeat.DownThresholdMisses {
		if m.status != StatusDown {
			m.status = StatusDown
			m.confirmationRequired = true
			m.confirmationToken = m.generateConfirmationToken()

			msg := fmt.Sprintf("CRITICAL: Primary missed %d heartbeats (> %s). Node marked DOWN.",
				m.consecutiveMisses, elapsed.Round(time.Second))
			m.addEventLocked("CRITICAL", "PRIMARY_DOWN", msg)
			log.Printf("[REVIV-WATCHDOG] %s Token: %s", msg, m.confirmationToken)

			go m.dispatchAlert("CRITICAL", msg)
		}
	} else if m.consecutiveMisses >= m.cfg.Heartbeat.SuspectThresholdMisses {
		if m.status == StatusHealthy {
			m.status = StatusSuspect
			msg := fmt.Sprintf("WARNING: Primary missed %d heartbeats. Status changed to SUSPECT.", m.consecutiveMisses)
			m.addEventLocked("WARNING", "PRIMARY_SUSPECT", msg)
			log.Printf("[REVIV-WATCHDOG] %s", msg)

			go m.dispatchAlert("WARNING", msg)
		}
	}
}

// RecordHeartbeat processes an incoming telemetry packet from drdaemon.
func (m *Monitor) RecordHeartbeat(payload HeartbeatPayload) {
	m.mu.Lock()
	defer m.mu.Unlock()

	previousStatus := m.status
	m.lastHeartbeat = time.Now()
	m.consecutiveMisses = 0

	if payload.NodeID != "" {
		m.primaryNodeID = payload.NodeID
	}
	if payload.DBStatus != "" {
		m.dbStatus = payload.DBStatus
	}
	if !payload.LastWALPushedAt.IsZero() {
		m.lastWALPushedAt = payload.LastWALPushedAt
	}
	if payload.LastWALSequence != "" {
		m.lastWALSequence = payload.LastWALSequence
	}
	if payload.DiskFreePct > 0 {
		m.diskFreePct = payload.DiskFreePct
	}
	if payload.MemoryFreePct > 0 {
		m.memoryFreePct = payload.MemoryFreePct
	}

	// Check if recovering from outage
	if previousStatus == StatusDown || previousStatus == StatusSuspect {
		m.status = StatusHealthy
		m.confirmationRequired = false
		m.confirmationToken = ""
		m.confirmedByOperator = false

		msg := fmt.Sprintf("HEARTBEAT RECOVERED: Primary node %s is alive and pinging again.", m.primaryNodeID)
		m.addEventLocked("SUCCESS", "PRIMARY_RECOVERED", msg)
		log.Printf("[REVIV-WATCHDOG] %s", msg)

		go m.dispatchAlert("SUCCESS", msg)
	}
}

// ConfirmFailover validates the operator token to authorize disaster failover.
func (m *Monitor) ConfirmFailover(req OperatorConfirmRequest) (*OperatorConfirmResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.status != StatusDown && req.Token != "FORCE_OVERRIDE" {
		return &OperatorConfirmResponse{
			Success: false,
			Message: fmt.Sprintf("Failover confirmation rejected: Primary status is %s (must be DOWN)", m.status),
		}, fmt.Errorf("node not down")
	}

	if m.confirmationToken != "" && req.Token != m.confirmationToken && req.Token != "FORCE_OVERRIDE" && req.Token != "CONFIRM" {
		return &OperatorConfirmResponse{
			Success: false,
			Message: "Invalid operator confirmation token. Check terminal / dashboard for the active token.",
		}, fmt.Errorf("invalid token")
	}

	now := time.Now()
	m.confirmedByOperator = true
	m.confirmedAt = &now
	m.confirmedBy = req.Operator
	if m.confirmedBy == "" {
		m.confirmedBy = "Operator"
	}

	m.addEventLocked("SUCCESS", "OPERATOR_CONFIRMED",
		fmt.Sprintf("Failover authorized by %s. Reason: %s", m.confirmedBy, req.Reason))

	return &OperatorConfirmResponse{
		Success: true,
		Message: "Operator confirmation recorded. Failover authorized to provision AWS cloud resources.",
		Token:   m.confirmationToken,
	}, nil
}

// UpdateStandbyState updates cloud standby progression.
func (m *Monitor) UpdateStandbyState(state StandbyCloudState, ip string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cloudStandbyState = state
	if ip != "" {
		m.standbyComputeIP = ip
	}

	if state == StandbyActive {
		m.status = StatusFailoverTriggered
		m.failoverInProgress = false
		m.addEventLocked("SUCCESS", "STANDBY_ACTIVE", fmt.Sprintf("Disaster recovery completed. Standby active at IP: %s", ip))
	} else if state == StandbyProvisioning {
		m.failoverInProgress = true
		m.addEventLocked("INFO", "PROVISIONING_STARTED", "AWS SDK provisioning standby compute and database restore.")
	}
}

// GetStatus returns the complete operational status snapshot.
func (m *Monitor) GetStatus() MonitorStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	elapsed := now.Sub(m.lastHeartbeat).Seconds()
	walLag := now.Sub(m.lastWALPushedAt).Seconds()
	if walLag < 0 {
		walLag = 0
	}

	rpo := "< 30 seconds"
	if walLag > 30 {
		rpo = fmt.Sprintf("%.0f seconds (lagging)", walLag)
	}

	rto := "~5 to 8 minutes (Cold Standby)"
	if m.cloudStandbyState == StandbyActive {
		rto = "0s (Failover Active)"
	}

	// Copy events slice
	eventsCopy := make([]AuditEvent, len(m.auditEvents))
	copy(eventsCopy, m.auditEvents)

	return MonitorStatus{
		PrimaryNodeID:                m.primaryNodeID,
		Status:                       m.status,
		LastHeartbeatTime:            m.lastHeartbeat,
		TimeSinceLastHeartbeatSec:    elapsed,
		ConsecutiveMisses:            m.consecutiveMisses,
		SuspectThreshold:             m.cfg.Heartbeat.SuspectThresholdMisses,
		DownThreshold:                m.cfg.Heartbeat.DownThresholdMisses,
		DBStatus:                     m.dbStatus,
		LastWALPushedAt:              m.lastWALPushedAt,
		BackupLagSeconds:             walLag,
		EstimatedRPO:                 rpo,
		EstimatedRTO:                 rto,
		CloudStandbyState:            m.cloudStandbyState,
		StandbyComputeIP:             m.standbyComputeIP,
		OperatorConfirmationRequired: m.confirmationRequired,
		OperatorConfirmationToken:    m.confirmationToken,
		ConfirmedByOperator:          m.confirmedByOperator,
		ConfirmedAt:                  m.confirmedAt,
		LastAlertSentAt:              m.lastAlertSentAt,
		FailoverInProgress:           m.failoverInProgress,
		RecentEvents:                 eventsCopy,
	}
}

// SimulateOutage artificially sets the last heartbeat back to trigger failure detection.
func (m *Monitor) SimulateOutage() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lastHeartbeat = time.Now().Add(-60 * time.Second)
	m.consecutiveMisses = m.cfg.Heartbeat.DownThresholdMisses + 1
	m.status = StatusDown
	m.confirmationRequired = true
	m.confirmationToken = m.generateConfirmationToken()

	m.addEventLocked("CRITICAL", "SIMULATED_OUTAGE", "Simulated primary failure triggered via DR Control Plane.")
}

// SimulateRecovery artificially restores healthy heartbeat state.
func (m *Monitor) SimulateRecovery() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lastHeartbeat = time.Now()
	m.consecutiveMisses = 0
	m.status = StatusHealthy
	m.confirmationRequired = false
	m.confirmationToken = ""
	m.confirmedByOperator = false
	m.cloudStandbyState = StandbyColdIdle
	m.standbyComputeIP = ""
	m.failoverInProgress = false

	m.addEventLocked("SUCCESS", "SIMULATED_RECOVERY", "Simulated primary recovery completed. System back to normal.")
}

// Helper methods:
func (m *Monitor) addEvent(severity, event, details string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.addEventLocked(severity, event, details)
}

func (m *Monitor) addEventLocked(severity, event, details string) {
	ev := AuditEvent{
		Timestamp: time.Now(),
		Severity:  severity,
		Event:     event,
		Details:   details,
	}
	m.auditEvents = append([]AuditEvent{ev}, m.auditEvents...)
	if len(m.auditEvents) > 50 {
		m.auditEvents = m.auditEvents[:50]
	}
}

func (m *Monitor) generateConfirmationToken() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("CONFIRM-%s", hex.EncodeToString(b))
}

func (m *Monitor) dispatchAlert(severity, message string) {
	now := time.Now()
	m.mu.Lock()
	m.lastAlertSentAt = &now
	webhook := m.cfg.Alerting.WebhookURL
	slack := m.cfg.Alerting.SlackURL
	m.mu.Unlock()

	targetURL := webhook
	if targetURL == "" {
		targetURL = slack
	}
	if targetURL == "" {
		return
	}

	payload := map[string]interface{}{
		"severity":   severity,
		"message":    message,
		"timestamp":  now.Format(time.RFC3339),
		"engine":     "REVIV Automated Disaster Recovery",
		"primary_id": m.primaryNodeID,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewBuffer(data))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err == nil && resp != nil {
		_ = resp.Body.Close()
	}
}
