package config

import (
	"os"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Primary.NodeID == "" {
		t.Fatal("expected default primary node_id")
	}
	if cfg.Backup.S3Bucket == "" {
		t.Fatal("expected default S3 bucket")
	}
	if cfg.Heartbeat.DownThresholdMisses != 5 {
		t.Fatalf("expected DownThresholdMisses 5, got %d", cfg.Heartbeat.DownThresholdMisses)
	}
}

func TestEnvOverrides(t *testing.T) {
	os.Setenv("REVIV_S3_BUCKET", "custom-bucket-test")
	defer os.Unsetenv("REVIV_S3_BUCKET")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Backup.S3Bucket != "custom-bucket-test" {
		t.Fatalf("expected S3Bucket custom-bucket-test, got %s", cfg.Backup.S3Bucket)
	}
}
