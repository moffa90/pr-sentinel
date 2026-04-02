package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/moffa90/pr-sentinel/internal/config"
)

// HealthStatus represents the daemon's last known state.
type HealthStatus struct {
	LastPoll   time.Time `json:"last_poll"`
	CycleCount int       `json:"cycle_count"`
	LastErrors int       `json:"last_errors"`
	PID        int       `json:"pid"`
}

// HealthPath returns the path to the health status file.
func HealthPath() string {
	return filepath.Join(config.ConfigDir(), "health.json")
}

// WriteHealth writes the current health status to disk.
func WriteHealth(h HealthStatus) error {
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(HealthPath(), data, 0o600)
}

// ReadHealth reads the health status from disk.
func ReadHealth() (HealthStatus, error) {
	data, err := os.ReadFile(HealthPath())
	if err != nil {
		return HealthStatus{}, err
	}
	var h HealthStatus
	return h, json.Unmarshal(data, &h)
}
