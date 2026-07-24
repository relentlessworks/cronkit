package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/relentlessworks/cronkit/internal/auth"
	"github.com/relentlessworks/cronkit/internal/model"
	"github.com/relentlessworks/cronkit/internal/store"
)

func setupTest(t *testing.T) (*Handlers, *store.Store, string, string) {
	t.Helper()
	s, err := store.New(t.TempDir() + "/test.json")
	if err != nil {
		t.Fatalf("failed to create store: %s", err)
	}
	a := auth.New(s, "test-secret", "", "", "", "", "test@cronkit.local")
	sched := NewScheduler(s)
	h := NewHandlers(s, a, sched)
	sched.SetHandlers(h)

	// Create a workspace and token
	wsHandle := model.NewWorkspaceHandle()
	ws := &model.Workspace{
		Handle:    wsHandle,
		Name:      "test@example.com",
		Plan:      "free",
		CreatedAt: time.Now(),
	}
	s.CreateWorkspace(ws)
	a.SaveToken("test-token", wsHandle, "test@example.com")

	return h, s, "test-token", wsHandle
}

func TestHelp(t *testing.T) {
	h, _, _, _ := setupTest(t)
	req := httptest.NewRequest("GET", "/help", nil)
	w := httptest.NewRecorder()
	h.handleHelp(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "cronkit") {
		t.Error("help should mention cronkit")
	}
	if !strings.Contains(body, "POST /auth/request") {
		t.Error("help should mention auth flow")
	}
	if !strings.Contains(body, "POST /jobs") {
		t.Error("help should mention jobs endpoint")
	}
}

func TestAuthRequest(t *testing.T) {
	h, _, _, _ := setupTest(t)

	// Missing email
	req := httptest.NewRequest("POST", "/auth/request", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.handleAuthRequest(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing email, got %d", w.Code)
	}

	// Valid email
	form := url.Values{"email": {"agent@test.com"}}
	req = httptest.NewRequest("POST", "/auth/request", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	h.handleAuthRequest(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "otp_sent") {
		t.Error("should return otp_sent status")
	}
}

func TestAuthVerify(t *testing.T) {
	h, s, _, _ := setupTest(t)

	// Request OTP first
	form := url.Values{"email": {"verify@test.com"}}
	req := httptest.NewRequest("POST", "/auth/request", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.handleAuthRequest(w, req)

	// Get the OTP from store
	otp, ok := s.GetOTP("verify@test.com")
	if !ok {
		t.Fatal("OTP not found in store")
	}

	// Verify with correct code
	form = url.Values{"email": {"verify@test.com"}, "code": {otp.Code}}
	req = httptest.NewRequest("POST", "/auth/verify", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	h.handleAuthVerify(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "token=") {
		t.Error("should return a token")
	}

	// Verify with wrong code
	form = url.Values{"email": {"verify@test.com"}, "code": {"000000"}}
	req = httptest.NewRequest("POST", "/auth/verify", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	h.handleAuthVerify(w, req)
	if w.Code != http.StatusOK {
		// OTP was consumed, so this should fail
	}
}

func TestCreateJob(t *testing.T) {
	h, _, token, _ := setupTest(t)

	form := url.Values{
		"name":     {"test-job"},
		"schedule": {"5m"},
		"url":      {"https://httpbin.org/post"},
		"method":   {"POST"},
	}
	req := httptest.NewRequest("POST", "/jobs", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.handleJobs(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "handle=job_") {
		t.Error("should return a job handle")
	}
	if !strings.Contains(body, "schedule=5m") {
		t.Error("should return the schedule")
	}
	if !strings.Contains(body, "type=interval") {
		t.Error("should detect interval type")
	}
}

func TestCreateJobCron(t *testing.T) {
	h, _, token, _ := setupTest(t)

	form := url.Values{
		"name":     {"cron-job"},
		"schedule": {"*/5 * * * *"},
		"url":      {"https://httpbin.org/post"},
		"method":   {"POST"},
	}
	req := httptest.NewRequest("POST", "/jobs", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.handleJobs(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "type=cron") {
		t.Error("should detect cron type")
	}
}

func TestCreateJobMissingFields(t *testing.T) {
	h, _, token, _ := setupTest(t)

	// Missing name
	form := url.Values{"schedule": {"5m"}, "url": {"https://example.com"}}
	req := httptest.NewRequest("POST", "/jobs", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.handleJobs(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing name, got %d", w.Code)
	}

	// Missing schedule
	form = url.Values{"name": {"test"}, "url": {"https://example.com"}}
	req = httptest.NewRequest("POST", "/jobs", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	h.handleJobs(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing schedule, got %d", w.Code)
	}

	// Missing URL
	form = url.Values{"name": {"test"}, "schedule": {"5m"}}
	req = httptest.NewRequest("POST", "/jobs", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	h.handleJobs(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing url, got %d", w.Code)
	}
}

func TestCreateJobInvalidSchedule(t *testing.T) {
	h, _, token, _ := setupTest(t)

	form := url.Values{
		"name":     {"bad-job"},
		"schedule": {"not-a-schedule"},
		"url":      {"https://example.com"},
	}
	req := httptest.NewRequest("POST", "/jobs", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.handleJobs(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid schedule, got %d", w.Code)
	}
}

func TestListJobs(t *testing.T) {
	h, _, token, _ := setupTest(t)

	// Create a job first
	form := url.Values{
		"name":     {"list-test"},
		"schedule": {"10m"},
		"url":      {"https://example.com"},
	}
	req := httptest.NewRequest("POST", "/jobs", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.handleJobs(w, req)

	// List jobs
	req = httptest.NewRequest("GET", "/jobs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	h.handleJobs(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "list-test") {
		t.Error("should list the created job")
	}
}

func TestGetJob(t *testing.T) {
	h, _, token, _ := setupTest(t)

	// Create a job
	form := url.Values{
		"name":     {"get-test"},
		"schedule": {"5m"},
		"url":      {"https://example.com"},
	}
	req := httptest.NewRequest("POST", "/jobs", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.handleJobs(w, req)

	// Extract handle from response
	body := w.Body.String()
	handleIdx := strings.Index(body, "handle=job_")
	if handleIdx < 0 {
		t.Fatal("could not find handle in response")
	}
	handle := body[handleIdx+7:]
	handle = handle[:strings.IndexAny(handle, " \n")]

	// Get the job
	req = httptest.NewRequest("GET", "/jobs/"+handle, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	h.handleJobByHandle(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "get-test") {
		t.Error("should return the job")
	}
}

func TestUpdateJob(t *testing.T) {
	h, _, token, _ := setupTest(t)

	// Create a job
	form := url.Values{
		"name":     {"update-test"},
		"schedule": {"5m"},
		"url":      {"https://example.com"},
	}
	req := httptest.NewRequest("POST", "/jobs", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.handleJobs(w, req)

	body := w.Body.String()
	handleIdx := strings.Index(body, "handle=job_")
	if handleIdx < 0 {
		t.Fatal("could not find handle in response")
	}
	handle := body[handleIdx+7:]
	handle = handle[:strings.IndexAny(handle, " \n")]

	// Update the job
	form = url.Values{"name": {"updated-name"}, "enabled": {"false"}}
	req = httptest.NewRequest("PATCH", "/jobs/"+handle, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	h.handleJobByHandle(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "updated-name") {
		t.Error("should show updated name")
	}
	if !strings.Contains(w.Body.String(), "enabled=false") {
		t.Error("should show disabled state")
	}
}

func TestDeleteJob(t *testing.T) {
	h, _, token, _ := setupTest(t)

	// Create a job
	form := url.Values{
		"name":     {"delete-test"},
		"schedule": {"5m"},
		"url":      {"https://example.com"},
	}
	req := httptest.NewRequest("POST", "/jobs", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.handleJobs(w, req)

	body := w.Body.String()
	handleIdx := strings.Index(body, "handle=job_")
	if handleIdx < 0 {
		t.Fatal("could not find handle in response")
	}
	handle := body[handleIdx+7:]
	handle = handle[:strings.IndexAny(handle, " \n")]

	// Delete the job
	req = httptest.NewRequest("DELETE", "/jobs/"+handle, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	h.handleJobByHandle(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "deleted") {
		t.Error("should confirm deletion")
	}

	// Verify it's gone
	req = httptest.NewRequest("GET", "/jobs/"+handle, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	h.handleJobByHandle(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", w.Code)
	}
}

func TestJSONResponse(t *testing.T) {
	h, _, token, _ := setupTest(t)

	// Create a job
	form := url.Values{
		"name":     {"json-test"},
		"schedule": {"5m"},
		"url":      {"https://example.com"},
	}
	req := httptest.NewRequest("POST", "/jobs", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	h.handleJobs(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}

	var job map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&job); err != nil {
		t.Errorf("failed to decode JSON: %s", err)
	}
	if job["name"] != "json-test" {
		t.Errorf("expected name=json-test, got %v", job["name"])
	}
}

func TestHealth(t *testing.T) {
	h, _, _, _ := setupTest(t)
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "status=ok") {
		t.Error("should return ok status")
	}
}

func TestAuthRequired(t *testing.T) {
	h, _, _, _ := setupTest(t)

	// No auth header — use middleware-wrapped handler
	handler := h.middleware.RequireAuth(h.handleJobs)

	req := httptest.NewRequest("GET", "/jobs", nil)
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", w.Code)
	}

	// Invalid token
	req = httptest.NewRequest("GET", "/jobs", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w = httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with invalid token, got %d", w.Code)
	}
}

func TestCronParsing(t *testing.T) {
	tests := []struct {
		expr    string
		wantErr bool
	}{
		{"*/5 * * * *", false},
		{"0 * * * *", false},
		{"0 9 * * 1-5", false},
		{"0 0 * * 0", false},
		{"30 14 1 * *", false},
		{"0,30 * * * *", false},
		{"0-59/15 * * * *", false},
		{"* * * * *", false},
		{"invalid", true},
		{"* * * *", true},
		{"60 * * * *", true},
		{"* 25 * * *", true},
	}

	for _, tt := range tests {
		_, err := model.ParseCron(tt.expr)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseCron(%q) error = %v, wantErr %v", tt.expr, err, tt.wantErr)
		}
	}
}

func TestCronNext(t *testing.T) {
	// Every minute
	c, _ := model.ParseCron("* * * * *")
	from := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	next := c.Next(from)
	expected := time.Date(2026, 1, 1, 12, 1, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, next)
	}

	// Every 5 minutes
	c, _ = model.ParseCron("*/5 * * * *")
	from = time.Date(2026, 1, 1, 12, 2, 0, 0, time.UTC)
	next = c.Next(from)
	expected = time.Date(2026, 1, 1, 12, 5, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, next)
	}

	// 9am on weekdays
	c, _ = model.ParseCron("0 9 * * 1-5")
	from = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC) // Thursday 10am
	next = c.Next(from)
	expected = time.Date(2026, 1, 2, 9, 0, 0, 0, time.UTC) // Friday 9am
	if !next.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, next)
	}
}

func TestIntervalParsing(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"30s", false},
		{"5m", false},
		{"1h", false},
		{"2h30m", false},
		{"1h30m45s", false},
		{"", true},
		{"invalid", true},
	}

	for _, tt := range tests {
		_, err := model.ParseInterval(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseInterval(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
		}
	}
}

func TestValidateSchedule(t *testing.T) {
	// Interval
	st, err := model.ValidateSchedule("5m")
	if err != nil || st != "interval" {
		t.Errorf("expected interval type, got %s, err=%v", st, err)
	}

	// Cron
	st, err = model.ValidateSchedule("*/5 * * * *")
	if err != nil || st != "cron" {
		t.Errorf("expected cron type, got %s, err=%v", st, err)
	}

	// Invalid
	_, err = model.ValidateSchedule("not-valid")
	if err == nil {
		t.Error("expected error for invalid schedule")
	}
}

func TestCalculateNextRun(t *testing.T) {
	from := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	// Interval
	next, err := model.CalculateNextRun("5m", "interval", from)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := from.Add(5 * time.Minute)
	if !next.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, next)
	}

	// Cron
	next, err = model.CalculateNextRun("* * * * *", "cron", from)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected = time.Date(2026, 1, 1, 12, 1, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, next)
	}
}

func TestHandleGeneration(t *testing.T) {
	h1 := model.NewJobHandle()
	h2 := model.NewJobHandle()
	if h1 == h2 {
		t.Error("handles should be unique")
	}
	if !strings.HasPrefix(h1, "job_") {
		t.Errorf("job handle should start with 'job_', got %s", h1)
	}
	if !strings.HasPrefix(model.NewWorkspaceHandle(), "ws_") {
		t.Error("workspace handle should start with 'ws_'")
	}
}

func TestRunLogs(t *testing.T) {
	h, s, token, wsHandle := setupTest(t)

	// Create a job
	form := url.Values{
		"name":     {"log-test"},
		"schedule": {"5m"},
		"url":      {"https://example.com"},
	}
	req := httptest.NewRequest("POST", "/jobs", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Workspace", wsHandle)
	w := httptest.NewRecorder()
	h.handleJobs(w, req)

	body := w.Body.String()
	handleIdx := strings.Index(body, "handle=job_")
	if handleIdx < 0 {
		t.Fatal("could not find handle in response")
	}
	handle := body[handleIdx+7:]
	handle = handle[:strings.IndexAny(handle, " \n")]

	// Add a run log manually
	s.AddRunLog(model.RunLog{
		JobHandle:  handle,
		Workspace:  wsHandle,
		Status:     "success",
		StatusCode: 200,
		Duration:   150,
		StartedAt:  time.Now(),
		FinishedAt: time.Now(),
	})

	// List runs
	req = httptest.NewRequest("GET", "/runs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Workspace", wsHandle)
	w = httptest.NewRecorder()
	h.handleRuns(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "success") {
		t.Error("should list the run log")
	}

	// List runs for specific job
	req = httptest.NewRequest("GET", "/runs/"+handle, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Workspace", wsHandle)
	w = httptest.NewRecorder()
	h.handleRunsByJob(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), handle) {
		t.Error("should list runs for the job")
	}
}

func TestWorkspaceCreation(t *testing.T) {
	h, _, token, _ := setupTest(t)

	form := url.Values{"name": {"my-new-workspace"}}
	req := httptest.NewRequest("POST", "/workspaces", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.handleWorkspaces(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "handle=ws_") {
		t.Error("should return a workspace handle")
	}
}

func TestErrorFormat(t *testing.T) {
	h, _, _, _ := setupTest(t)

	// Test error format in plain text
	req := httptest.NewRequest("POST", "/auth/request", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.handleAuthRequest(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "error:") {
		t.Error("error should contain 'error:' prefix")
	}
	if !strings.Contains(body, "hint:") {
		t.Error("error should contain 'hint:' for self-correction")
	}
}
