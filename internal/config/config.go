package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the complete REVIV disaster recovery configuration.
type Config struct {
	Primary   PrimaryConfig   `yaml:"primary"`
	Backup    BackupConfig    `yaml:"backup"`
	Heartbeat HeartbeatConfig `yaml:"heartbeat"`
	Alerting  AlertingConfig  `yaml:"alerting"`
	AWS       AWSConfig       `yaml:"aws"`
}

// PrimaryConfig defines parameters for the primary workload (e.g. VPS).
type PrimaryConfig struct {
	NodeID                  string `yaml:"node_id"`
	Host                    string `yaml:"host"`
	HeartbeatURL            string `yaml:"heartbeat_url"`
	HeartbeatIntervalSecond int    `yaml:"heartbeat_interval_seconds"`
	DBHost                  string `yaml:"db_host"`
	DBPort                  int    `yaml:"db_port"`
	DBName                  string `yaml:"db_name"`
	DBUser                  string `yaml:"db_user"`
	DBPassword              string `yaml:"db_password"`
}

// BackupConfig defines WAL-G and S3 backup options.
type BackupConfig struct {
	S3Bucket               string `yaml:"s3_bucket"`
	S3Region               string `yaml:"s3_region"`
	WALPrefix              string `yaml:"wal_prefix"`
	WALPushIntervalSeconds int    `yaml:"wal_push_interval_seconds"`
	BaseBackupCron         string `yaml:"base_backup_cron"`
}

// HeartbeatConfig defines failure detection thresholds.
type HeartbeatConfig struct {
	ListenPort               int           `yaml:"listen_port"`
	CheckInterval            time.Duration `yaml:"check_interval"`
	SuspectThresholdMisses   int           `yaml:"suspect_threshold_misses"`
	DownThresholdMisses      int           `yaml:"down_threshold_misses"`
	TimeoutSeconds           int           `yaml:"timeout_seconds"`
	RequireConfirmation      bool          `yaml:"require_confirmation"`
	ConfirmationTimeoutHours int           `yaml:"confirmation_timeout_hours"`
}

// AlertingConfig defines webhook notification targets.
type AlertingConfig struct {
	Enabled    bool   `yaml:"enabled"`
	WebhookURL string `yaml:"webhook_url"`
	SlackURL   string `yaml:"slack_url"`
}

// AWSConfig defines recovery cloud parameters.
type AWSConfig struct {
	Region           string `yaml:"region"`
	VPCID            string `yaml:"vpc_id"`
	SubnetID         string `yaml:"subnet_id"`
	SecurityGroupID  string `yaml:"security_group_id"`
	AMIID            string `yaml:"ami_id"`
	InstanceType     string `yaml:"instance_type"`
	KeyPairName      string `yaml:"key_pair_name"`
	Route53ZoneID    string `yaml:"route53_zone_id"`
	Domain           string `yaml:"domain"`
	RecordType       string `yaml:"record_type"`
	DockerImage      string `yaml:"docker_image"`
	SimulateCloudOps bool   `yaml:"simulate_cloud_ops"` // Useful for local testing without AWS credit charges
}

// DefaultConfig returns safe, preconfigured production-ready defaults.
func DefaultConfig() *Config {
	return &Config{
		Primary: PrimaryConfig{
			NodeID:                  "primary-vps-01",
			Host:                    "127.0.0.1",
			HeartbeatURL:            "http://127.0.0.1:8080/api/heartbeat",
			HeartbeatIntervalSecond: 5,
			DBHost:                  "127.0.0.1",
			DBPort:                  5432,
			DBName:                  "mydb",
			DBUser:                  "myuser",
			DBPassword:              "mypassword",
		},
		Backup: BackupConfig{
			S3Bucket:               "reviv-dr-backups",
			S3Region:               "ap-south-1",
			WALPrefix:              "wal/",
			WALPushIntervalSeconds: 30,
			BaseBackupCron:         "0 3 * * *",
		},
		Heartbeat: HeartbeatConfig{
			ListenPort:               8080,
			CheckInterval:            5 * time.Second,
			SuspectThresholdMisses:   3,
			DownThresholdMisses:      5,
			TimeoutSeconds:           30,
			RequireConfirmation:      true,
			ConfirmationTimeoutHours: 2,
		},
		Alerting: AlertingConfig{
			Enabled:    true,
			WebhookURL: "",
			SlackURL:   "",
		},
		AWS: AWSConfig{
			Region:           "ap-south-1",
			InstanceType:     "t3.small",
			Route53ZoneID:    "Z1001REVIVEXAMPLE",
			Domain:           "app.reviv-dr.internal",
			RecordType:       "A",
			DockerImage:      "customer/app:latest",
			SimulateCloudOps: true, // Default to simulation mode unless real AWS credentials explicitly engage
		},
	}
}

// LoadConfig loads configuration from a YAML file, falling back to defaults and env vars.
func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path != "" {
		data, err := os.ReadFile(path)
		if err == nil {
			if err := yaml.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("failed to parse config YAML: %w", err)
			}
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
		}
	}

	// Environment variable overrides
	if s3Bucket := os.Getenv("REVIV_S3_BUCKET"); s3Bucket != "" {
		cfg.Backup.S3Bucket = s3Bucket
	}
	if s3Region := os.Getenv("AWS_REGION"); s3Region != "" {
		cfg.AWS.Region = s3Region
		cfg.Backup.S3Region = s3Region
	}
	if webhook := os.Getenv("REVIV_WEBHOOK_URL"); webhook != "" {
		cfg.Alerting.WebhookURL = webhook
		cfg.Alerting.Enabled = true
	}
	if simulate := os.Getenv("REVIV_SIMULATE_AWS"); simulate != "" {
		val, _ := strconv.ParseBool(simulate)
		cfg.AWS.SimulateCloudOps = val
	}

	return cfg, nil
}
