package heartbeat

import "time"

// NodeStatus defines the operational health state of the monitored primary node.
type NodeStatus string

const (
	StatusHealthy           NodeStatus = "HEALTHY"
	StatusSuspect           NodeStatus = "SUSPECT"
	StatusDown              NodeStatus = "DOWN"
	StatusRecovering        NodeStatus = "RECOVERING"
	StatusFailoverTriggered NodeStatus = "FAILOVER_TRIGGERED"
)

// StandbyCloudState defines the cloud cold-standby status.
type StandbyCloudState string

const (
	StandbyColdIdle     StandbyCloudState = "COLD_IDLE"
	StandbyProvisioning StandbyCloudState = "PROVISIONING_COMPUTE"
	StandbyRestoringDB  StandbyCloudState = "RESTORING_DATABASE"
	StandbySwitchingDNS StandbyCloudState = "SWITCHING_DNS"
	StandbyActive       StandbyCloudState = "STANDBY_ACTIVE"
)

// HeartbeatPayload represents the telemetry packet dispatched by drdaemon.
type HeartbeatPayload struct {
	NodeID          string    `json:"node_id"`
	Timestamp       time.Time `json:"timestamp"`
	DBStatus        string    `json:"db_status"` // UP, DOWN, DEGRADED
	LastWALPushedAt time.Time `json:"last_wal_pushed_at"`
	LastWALSequence string    `json:"last_wal_sequence"`
	DiskFreePct     float64   `json:"disk_free_pct"`
	MemoryFreePct   float64   `json:"memory_free_pct"`
	UptimeSeconds   int64     `json:"uptime_seconds"`
	Version         string    `json:"version"`
}

// AuditEvent represents a timestamped operational event in REVIV.
type AuditEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Severity  string    `json:"severity"` // INFO, WARNING, CRITICAL, SUCCESS
	Event     string    `json:"event"`
	Details   string    `json:"details"`
}

// MonitorStatus represents the live state of the DR control engine.
type MonitorStatus struct {
	PrimaryNodeID                string            `json:"primary_node_id"`
	Status                       NodeStatus        `json:"status"`
	LastHeartbeatTime            time.Time         `json:"last_heartbeat_time"`
	TimeSinceLastHeartbeatSec    float64           `json:"time_since_last_heartbeat_sec"`
	ConsecutiveMisses            int               `json:"consecutive_misses"`
	SuspectThreshold             int               `json:"suspect_threshold"`
	DownThreshold                int               `json:"down_threshold"`
	DBStatus                     string            `json:"db_status"`
	LastWALPushedAt              time.Time         `json:"last_wal_pushed_at"`
	BackupLagSeconds             float64           `json:"backup_lag_seconds"`
	EstimatedRPO                 string            `json:"estimated_rpo"`
	EstimatedRTO                 string            `json:"estimated_rto"`
	CloudStandbyState            StandbyCloudState `json:"cloud_standby_state"`
	StandbyComputeIP             string            `json:"standby_compute_ip,omitempty"`
	OperatorConfirmationRequired bool              `json:"operator_confirmation_required"`
	OperatorConfirmationToken    string            `json:"operator_confirmation_token,omitempty"`
	ConfirmedByOperator          bool              `json:"confirmed_by_operator"`
	ConfirmedAt                  *time.Time        `json:"confirmed_at,omitempty"`
	LastAlertSentAt              *time.Time        `json:"last_alert_sent_at,omitempty"`
	FailoverInProgress           bool              `json:"failover_in_progress"`
	RecentEvents                 []AuditEvent      `json:"recent_events"`
}

// OperatorConfirmRequest payload for confirming disaster failover.
type OperatorConfirmRequest struct {
	Token    string `json:"token"`
	Operator string `json:"operator"`
	Reason   string `json:"reason"`
}

// OperatorConfirmResponse result of confirmation validation.
type OperatorConfirmResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Token   string `json:"token"`
}
