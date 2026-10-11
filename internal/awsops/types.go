package awsops

import "time"

// StandbyComputeResult represents the cloud compute resource created upon failover.
type StandbyComputeResult struct {
	InstanceID   string    `json:"instance_id"`
	PublicIP     string    `json:"public_ip"`
	PrivateIP    string    `json:"private_ip"`
	InstanceType string    `json:"instance_type"`
	State        string    `json:"state"`
	LaunchedAt   time.Time `json:"launched_at"`
	IsSimulated  bool      `json:"is_simulated"`
}

// Route53SwitchResult represents the DNS cutover outcome.
type Route53SwitchResult struct {
	ZoneID      string    `json:"zone_id"`
	Domain      string    `json:"domain"`
	RecordType  string    `json:"record_type"`
	NewTarget   string    `json:"new_target"`
	ChangeID    string    `json:"change_id"`
	Status      string    `json:"status"`
	SwitchedAt  time.Time `json:"switched_at"`
	IsSimulated bool      `json:"is_simulated"`
}

// FailoverStepLog records an individual phase of the automated DR failover.
type FailoverStepLog struct {
	StepNumber  int           `json:"step_number"`
	StepName    string        `json:"step_name"`
	Status      string        `json:"status"` // IN_PROGRESS, COMPLETED, FAILED
	Description string        `json:"description"`
	Duration    time.Duration `json:"duration"`
	Timestamp   time.Time     `json:"timestamp"`
}

// FailoverExecutionReport contains the complete post-mortem and audit record of failover.
type FailoverExecutionReport struct {
	PrimaryNodeID        string            `json:"primary_node_id"`
	TriggeredAt          time.Time         `json:"triggered_at"`
	CompletedAt          time.Time         `json:"completed_at"`
	TotalRTODuration     time.Duration     `json:"total_rto_duration"`
	AchievedRTOFormatted string            `json:"achieved_rto_formatted"`
	AchievedRPOFormatted string            `json:"achieved_rpo_formatted"`
	OperatorConfirmedBy  string            `json:"operator_confirmed_by"`
	Success              bool              `json:"success"`
	ErrorMessage         string            `json:"error_message,omitempty"`
	ComputeResult        StandbyComputeResult `json:"compute_result"`
	DNSResult            Route53SwitchResult  `json:"dns_result"`
	Steps                []FailoverStepLog `json:"steps"`
}
