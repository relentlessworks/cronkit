package model

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strings"
	"time"
)

// Job represents a scheduled job.
type Job struct {
	Handle     string    `json:"handle"`
	Workspace  string    `json:"workspace"`
	Name       string    `json:"name"`
	Schedule   string    `json:"schedule"`   // cron expression or interval
	ScheduleType string  `json:"schedule_type"` // "cron" or "interval"
	URL        string    `json:"url"`        // webhook URL to call
	Method     string    `json:"method"`     // HTTP method (GET, POST, etc.)
	Headers    map[string]string `json:"headers,omitempty"`
	Body       string    `json:"body,omitempty"`
	Enabled    bool      `json:"enabled"`
	LastRun    *time.Time `json:"last_run,omitempty"`
	NextRun    time.Time `json:"next_run"`
	RunCount   int       `json:"run_count"`
	MaxRetries int       `json:"max_retries"`
	Timeout    int       `json:"timeout"` // seconds
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// RunLog represents a single execution of a job.
type RunLog struct {
	ID         int64     `json:"id"`
	JobHandle  string    `json:"job_handle"`
	Workspace  string    `json:"workspace"`
	Status     string    `json:"status"` // "success", "failed", "timeout"
	StatusCode int       `json:"status_code"`
	Duration   int64     `json:"duration_ms"`
	Error      string    `json:"error,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
}

// Workspace represents a tenant.
type Workspace struct {
	Handle    string    `json:"handle"`
	Name      string    `json:"name"`
	Plan      string    `json:"plan"`
	CreatedAt time.Time `json:"created_at"`
}

// Token represents an auth token.
type Token struct {
	Token       string    `json:"token"`
	Workspace   string    `json:"workspace"`
	Email       string    `json:"email"`
	CreatedAt   time.Time `json:"created_at"`
}

// OTP represents a one-time password.
type OTP struct {
	Email     string    `json:"email"`
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
}

// generateHandle creates a short stable handle like job_k7m2q.
func generateHandle(prefix string) string {
	b := make([]byte, 5)
	rand.Read(b)
	s := base32.StdEncoding.EncodeToString(b)
	s = strings.ToLower(strings.ReplaceAll(s, "=", ""))
	if len(s) > 5 {
		s = s[:5]
	}
	return fmt.Sprintf("%s_%s", prefix, s)
}

// NewJobHandle generates a job handle.
func NewJobHandle() string {
	return generateHandle("job")
}

// NewWorkspaceHandle generates a workspace handle.
func NewWorkspaceHandle() string {
	return generateHandle("ws")
}

// ParseInterval parses an interval string like "5m", "1h", "30s" into a Duration.
func ParseInterval(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty interval")
	}
	// Try standard Go duration parsing
	d, err := time.ParseDuration(s)
	if err == nil {
		return d, nil
	}
	return 0, fmt.Errorf("invalid interval format: %s (use formats like 30s, 5m, 1h, 2h30m)", s)
}
