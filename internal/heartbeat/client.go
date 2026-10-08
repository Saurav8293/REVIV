package heartbeat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client sends heartbeat telemetry from primary node (drdaemon) to drwatch.
type Client struct {
	targetURL  string
	httpClient *http.Client
}

// NewClient initializes a robust heartbeat HTTP client.
func NewClient(targetURL string) *Client {
	return &Client{
		targetURL: targetURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// SendHeartbeat sends a telemetry heartbeat packet to the monitor server.
func (c *Client) SendHeartbeat(ctx context.Context, payload HeartbeatPayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal heartbeat payload failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.targetURL, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("create heartbeat request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("dispatch heartbeat failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected heartbeat response status: %s", resp.Status)
	}

	return nil
}
