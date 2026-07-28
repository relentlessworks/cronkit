package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/relentlessworks/cronkit/internal/auth"
	"github.com/relentlessworks/cronkit/internal/model"
	"github.com/relentlessworks/cronkit/internal/store"
)

// Handlers holds all HTTP handlers.
type Handlers struct {
	store      *store.Store
	auth       *auth.Auth
	middleware *Middleware
	scheduler  SchedulerInterface
}

// SchedulerInterface defines what the scheduler needs to provide.
type SchedulerInterface interface {
	NotifyJobChange()
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(s *store.Store, a *auth.Auth, sched SchedulerInterface) *Handlers {
	return &Handlers{
		store:      s,
		auth:       a,
		middleware: NewMiddleware(a),
		scheduler:  sched,
	}
}

// SetupRoutes configures all routes on the given mux.
func (h *Handlers) SetupRoutes(mux *http.ServeMux) {
	// Public endpoints
	mux.HandleFunc("/help", h.handleHelp)
	mux.HandleFunc("/.well-known/agent.md", h.handleHelp)
	mux.HandleFunc("/auth/request", h.handleAuthRequest)
	mux.HandleFunc("/auth/verify", h.handleAuthVerify)

	// Authenticated endpoints
	mux.HandleFunc("/workspaces", h.middleware.RequireAuth(h.handleWorkspaces))
	mux.HandleFunc("/jobs", h.middleware.RequireAuth(h.handleJobs))
	mux.HandleFunc("/jobs/", h.middleware.RequireAuth(h.handleJobByHandle))
	mux.HandleFunc("/runs", h.middleware.RequireAuth(h.handleRuns))
	mux.HandleFunc("/runs/", h.middleware.RequireAuth(h.handleRunsByJob))
	mux.HandleFunc("/health", h.handleHealth)
}

// --- Help ---

func (h *Handlers) handleHelp(w http.ResponseWriter, r *http.Request) {
	help := `cronkit — agentic-first scheduled jobs and cron service

AUTH:
  1. POST /auth/request  body: email=<your-email>       — sends OTP code
  2. POST /auth/verify    body: email=<your-email>&code=<6-digit-code> — returns bearer token
  3. Use token in all subsequent requests: Authorization: Bearer <token>

WORKSPACES:
  POST /workspaces   body: name=<workspace-name>        — create a workspace
  GET  /workspaces                                     — list your workspaces

JOBS:
  POST /jobs   body: name=<name>&schedule=<expr>&url=<webhook-url>&method=POST
    schedule: cron expression (e.g. "*/5 * * * *") or interval (e.g. "5m", "1h", "30s")
    url: webhook URL to call when job fires
    method: HTTP method (default POST)
    body: optional request body
    headers[key]=value: optional custom headers (repeatable)
    enabled: true/false (default true)
    timeout: seconds (default 30)
    max_retries: number (default 3)
  GET  /jobs                                           — list all jobs
  GET  /jobs/<handle>                                  — get a specific job
  PATCH /jobs/<handle>                                 — update a job (same params as POST)
  DELETE /jobs/<handle>                                — delete a job
  POST /jobs/<handle>/trigger                          — manually trigger a job

RUN LOGS:
  GET /runs                                            — list recent run logs
  GET /runs/<job-handle>                               — list runs for a specific job

HEALTH:
  GET /health                                          — service health check

RESPONSE FORMAT:
  Plain text by default (one record per line, key=value pairs).
  Add Accept: application/json header or ?format=json for JSON.
  Errors: "error: <message> | hint: <what-to-do-next>"

SCHEDULE FORMATS:
  Cron:     "*/5 * * * *"     every 5 minutes
            "0 * * * *"       every hour
            "0 9 * * 1-5"     9am on weekdays
            "0 0 * * 0"       midnight on Sundays
  Interval: "30s"              every 30 seconds
            "5m"              every 5 minutes
            "1h"              every hour
            "2h30m"           every 2 hours 30 minutes

EXAMPLES:
  curl -X POST http://localhost:7777/auth/request -d 'email=agent@example.com'
  curl -X POST http://localhost:7777/auth/verify -d 'email=agent@example.com&code=123456'
  curl -X POST http://localhost:7777/workspaces -H "Authorization: Bearer <token>" -d 'name=my-team'
  curl -X POST http://localhost:7777/jobs -H "Authorization: Bearer <token>" -d 'name=daily-report&schedule=0 9 * * *&url=https://api.example.com/report&method=POST'
  curl http://localhost:7777/jobs -H "Authorization: Bearer <token>"
  curl -X POST http://localhost:7777/jobs/job_abc12/trigger -H "Authorization: Bearer <token>"
`
	writeText(w, http.StatusOK, help)
}

// --- Auth ---

func (h *Handlers) handleAuthRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use POST")
		return
	}

	email := r.FormValue("email")
	if email == "" {
		// Try JSON body
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			email = body["email"]
		}
	}
	if email == "" {
		writeError(w, r, http.StatusBadRequest, "email is required", "send email=<your-email> in the request body")
		return
	}

	if err := h.auth.RequestOTP(email); err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to send OTP", "check server logs or try again")
		return
	}

	writeRecord(w, r, http.StatusOK, "status=otp_sent email="+email, map[string]string{"status": "otp_sent", "email": email})
}

func (h *Handlers) handleAuthVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use POST")
		return
	}

	email := r.FormValue("email")
	code := r.FormValue("code")
	if email == "" {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			email = body["email"]
			code = body["code"]
		}
	}
	if email == "" || code == "" {
		writeError(w, r, http.StatusBadRequest, "email and code are required", "send email=<your-email>&code=<6-digit-code> in the request body")
		return
	}

	token, err := h.auth.VerifyOTP(email, code)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, err.Error(), "request a new OTP via POST /auth/request, then verify with POST /auth/verify")
		return
	}

	// Find or create workspace for this user
	wsHandle := ""
	if existingWS, ok := h.store.GetWorkspaceByName(email); ok {
		wsHandle = existingWS.Handle
	} else {
		wsHandle = model.NewWorkspaceHandle()
		ws := &model.Workspace{
			Handle:    wsHandle,
			Name:      email,
			Plan:      "free",
			CreatedAt: time.Now(),
		}
		h.store.CreateWorkspace(ws)
	}

	h.auth.SaveToken(token, wsHandle, email)

	writeRecord(w, r, http.StatusOK,
		fmt.Sprintf("token=%s workspace=%s", token, wsHandle),
		map[string]string{"token": token, "workspace": wsHandle})
}

// --- Workspaces ---

func (h *Handlers) handleWorkspaces(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.createWorkspace(w, r)
	case http.MethodGet:
		h.listWorkspaces(w, r)
	default:
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use GET or POST")
	}
}

func (h *Handlers) createWorkspace(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	if name == "" {
		writeError(w, r, http.StatusBadRequest, "name is required", "send name=<workspace-name> in the request body")
		return
	}

	if existing, ok := h.store.GetWorkspaceByName(name); ok {
		writeRecord(w, r, http.StatusOK,
			fmt.Sprintf("handle=%s name=%s plan=%s (already exists)", existing.Handle, existing.Name, existing.Plan),
			existing)
		return
	}

	handle := model.NewWorkspaceHandle()
	ws := &model.Workspace{
		Handle:    handle,
		Name:      name,
		Plan:      "free",
		CreatedAt: time.Now(),
	}

	if err := h.store.CreateWorkspace(ws); err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to create workspace", "try again or check server logs")
		return
	}

	writeRecord(w, r, http.StatusCreated,
		fmt.Sprintf("handle=%s name=%s plan=%s", ws.Handle, ws.Name, ws.Plan),
		ws)
}

func (h *Handlers) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	email := r.Header.Get("X-Email")
	// Find workspace by email (which was used as name during auto-create)
	ws, ok := h.store.GetWorkspaceByName(email)
	if !ok {
		writeRecords(w, r, http.StatusOK, nil, []interface{}{})
		return
	}
	writeRecord(w, r, http.StatusOK,
		fmt.Sprintf("handle=%s name=%s plan=%s", ws.Handle, ws.Name, ws.Plan),
		[]model.Workspace{*ws})
}

// --- Jobs ---

func (h *Handlers) handleJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.createJob(w, r)
	case http.MethodGet:
		h.listJobs(w, r)
	default:
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use GET or POST")
	}
}

func (h *Handlers) createJob(w http.ResponseWriter, r *http.Request) {
	workspace := r.Header.Get("X-Workspace")

	name := r.FormValue("name")
	schedule := r.FormValue("schedule")
	url := r.FormValue("url")

	if name == "" {
		writeError(w, r, http.StatusBadRequest, "name is required", "send name=<job-name> in the request body")
		return
	}
	if schedule == "" {
		writeError(w, r, http.StatusBadRequest, "schedule is required", "send schedule=<cron-expr-or-interval> e.g. schedule=*/5 * * * * or schedule=5m")
		return
	}
	if url == "" {
		writeError(w, r, http.StatusBadRequest, "url is required", "send url=<webhook-url> — the URL to call when the job fires")
		return
	}

	// Validate schedule
	scheduleType, err := model.ValidateSchedule(schedule)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err.Error(), "use a cron expression like '*/5 * * * *' or an interval like '5m', '1h', '30s'")
		return
	}

	method := r.FormValue("method")
	if method == "" {
		method = "POST"
	}

	enabled := true
	if v := r.FormValue("enabled"); v == "false" || v == "0" {
		enabled = false
	}

	timeout := 30
	if v := r.FormValue("timeout"); v != "" {
		if t, err := strconv.Atoi(v); err == nil && t > 0 {
			timeout = t
		}
	}

	maxRetries := 3
	if v := r.FormValue("max_retries"); v != "" {
		if m, err := strconv.Atoi(v); err == nil && m >= 0 {
			maxRetries = m
		}
	}

	// Parse headers from form
	headers := make(map[string]string)
	if err := r.ParseForm(); err == nil {
		for key, values := range r.Form {
			if strings.HasPrefix(key, "headers[") && strings.HasSuffix(key, "]") {
				headerKey := key[8 : len(key)-1]
				if len(values) > 0 {
					headers[headerKey] = values[0]
				}
			}
		}
	}

	body := r.FormValue("body")

	// Calculate next run
	nextRun, err := model.CalculateNextRun(schedule, scheduleType, time.Now())
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err.Error(), "check your schedule format")
		return
	}

	handle := model.NewJobHandle()
	job := &model.Job{
		Handle:       handle,
		Workspace:    workspace,
		Name:         name,
		Schedule:     schedule,
		ScheduleType: scheduleType,
		URL:          url,
		Method:       method,
		Headers:      headers,
		Body:         body,
		Enabled:      enabled,
		NextRun:      nextRun,
		MaxRetries:   maxRetries,
		Timeout:      timeout,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := h.store.CreateJob(job); err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to create job", "try again or check server logs")
		return
	}

	if h.scheduler != nil {
		h.scheduler.NotifyJobChange()
	}

	writeRecord(w, r, http.StatusCreated, formatJob(job), job)
}

func (h *Handlers) listJobs(w http.ResponseWriter, r *http.Request) {
	workspace := r.Header.Get("X-Workspace")
	jobs := h.store.ListJobs(workspace)

	var records []string
	for _, job := range jobs {
		records = append(records, formatJob(job))
	}

	writeRecords(w, r, http.StatusOK, records, jobs)
}

func (h *Handlers) handleJobByHandle(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/jobs/")
	parts := strings.SplitN(path, "/", 2)

	handle := parts[0]
	if handle == "" {
		writeError(w, r, http.StatusBadRequest, "job handle is required", "use GET /jobs/<handle> to get a specific job")
		return
	}

	// Check for /trigger sub-path
	if len(parts) == 2 && parts[1] == "trigger" {
		if r.Method != http.MethodPost {
			writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use POST to trigger a job")
			return
		}
		h.triggerJob(w, r, handle)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getJob(w, r, handle)
	case http.MethodPatch:
		h.updateJob(w, r, handle)
	case http.MethodDelete:
		h.deleteJob(w, r, handle)
	default:
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use GET, PATCH, or DELETE")
	}
}

func (h *Handlers) getJob(w http.ResponseWriter, r *http.Request, handle string) {
	workspace := r.Header.Get("X-Workspace")
	job, ok := h.store.GetJob(handle)
	if !ok || job.Workspace != workspace {
		writeError(w, r, http.StatusNotFound, "job not found", "check the handle with GET /jobs to list all jobs")
		return
	}
	writeRecord(w, r, http.StatusOK, formatJob(job), job)
}

func (h *Handlers) updateJob(w http.ResponseWriter, r *http.Request, handle string) {
	workspace := r.Header.Get("X-Workspace")
	job, ok := h.store.GetJob(handle)
	if !ok || job.Workspace != workspace {
		writeError(w, r, http.StatusNotFound, "job not found", "check the handle with GET /jobs to list all jobs")
		return
	}

	// Update fields if provided
	if v := r.FormValue("name"); v != "" {
		job.Name = v
	}
	if v := r.FormValue("schedule"); v != "" {
		scheduleType, err := model.ValidateSchedule(v)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, err.Error(), "use a cron expression like '*/5 * * * *' or an interval like '5m', '1h', '30s'")
			return
		}
		job.Schedule = v
		job.ScheduleType = scheduleType
		nextRun, err := model.CalculateNextRun(v, scheduleType, time.Now())
		if err != nil {
			writeError(w, r, http.StatusBadRequest, err.Error(), "check your schedule format")
			return
		}
		job.NextRun = nextRun
	}
	if v := r.FormValue("url"); v != "" {
		job.URL = v
	}
	if v := r.FormValue("method"); v != "" {
		job.Method = v
	}
	if v := r.FormValue("body"); v != "" {
		job.Body = v
	}
	if v := r.FormValue("enabled"); v != "" {
		if v == "false" || v == "0" {
			job.Enabled = false
		} else {
			job.Enabled = true
		}
	}
	if v := r.FormValue("timeout"); v != "" {
		if t, err := strconv.Atoi(v); err == nil && t > 0 {
			job.Timeout = t
		}
	}
	if v := r.FormValue("max_retries"); v != "" {
		if m, err := strconv.Atoi(v); err == nil && m >= 0 {
			job.MaxRetries = m
		}
	}

	// Update headers
	if err := r.ParseForm(); err == nil {
		for key, values := range r.Form {
			if strings.HasPrefix(key, "headers[") && strings.HasSuffix(key, "]") {
				headerKey := key[8 : len(key)-1]
				if len(values) > 0 {
					if job.Headers == nil {
						job.Headers = make(map[string]string)
					}
					job.Headers[headerKey] = values[0]
				}
			}
		}
	}

	if err := h.store.UpdateJob(job); err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to update job", "try again or check server logs")
		return
	}

	if h.scheduler != nil {
		h.scheduler.NotifyJobChange()
	}

	writeRecord(w, r, http.StatusOK, formatJob(job), job)
}

func (h *Handlers) deleteJob(w http.ResponseWriter, r *http.Request, handle string) {
	workspace := r.Header.Get("X-Workspace")
	job, ok := h.store.GetJob(handle)
	if !ok || job.Workspace != workspace {
		writeError(w, r, http.StatusNotFound, "job not found", "check the handle with GET /jobs to list all jobs")
		return
	}

	if err := h.store.DeleteJob(handle); err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to delete job", "try again or check server logs")
		return
	}

	if h.scheduler != nil {
		h.scheduler.NotifyJobChange()
	}

	writeRecord(w, r, http.StatusOK,
		fmt.Sprintf("deleted handle=%s name=%s", job.Handle, job.Name),
		map[string]string{"status": "deleted", "handle": job.Handle, "name": job.Name})
}

func (h *Handlers) triggerJob(w http.ResponseWriter, r *http.Request, handle string) {
	workspace := r.Header.Get("X-Workspace")
	job, ok := h.store.GetJob(handle)
	if !ok || job.Workspace != workspace {
		writeError(w, r, http.StatusNotFound, "job not found", "check the handle with GET /jobs to list all jobs")
		return
	}

	// Execute the job immediately in a goroutine
	go h.executeJob(job)

	writeRecord(w, r, http.StatusOK,
		fmt.Sprintf("triggered handle=%s name=%s url=%s", job.Handle, job.Name, job.URL),
		map[string]string{"status": "triggered", "handle": job.Handle, "name": job.Name})
}

// --- Run Logs ---

func (h *Handlers) handleRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use GET")
		return
	}

	workspace := r.Header.Get("X-Workspace")
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if l, err := strconv.Atoi(v); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	logs := h.store.ListRunLogs(workspace, "", limit)

	var records []string
	for _, log := range logs {
		records = append(records, formatRunLog(log))
	}

	writeRecords(w, r, http.StatusOK, records, logs)
}

func (h *Handlers) handleRunsByJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use GET")
		return
	}

	workspace := r.Header.Get("X-Workspace")
	jobHandle := strings.TrimPrefix(r.URL.Path, "/runs/")

	// Verify job exists and belongs to workspace
	job, ok := h.store.GetJob(jobHandle)
	if !ok || job.Workspace != workspace {
		writeError(w, r, http.StatusNotFound, "job not found", "check the handle with GET /jobs to list all jobs")
		return
	}

	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if l, err := strconv.Atoi(v); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	logs := h.store.ListRunLogs(workspace, jobHandle, limit)

	var records []string
	for _, log := range logs {
		records = append(records, formatRunLog(log))
	}

	writeRecords(w, r, http.StatusOK, records, logs)
}

// --- Health ---

func (h *Handlers) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeRecord(w, r, http.StatusOK, "status=ok service=cronkit", map[string]string{"status": "ok", "service": "cronkit"})
}

// --- Helpers ---

func formatJob(job *model.Job) string {
	lastRun := "never"
	if job.LastRun != nil {
		lastRun = job.LastRun.Format(time.RFC3339)
	}
	enabled := "true"
	if !job.Enabled {
		enabled = "false"
	}
	return fmt.Sprintf("handle=%s name=%s schedule=%s type=%s url=%s method=%s enabled=%s next_run=%s last_run=%s runs=%d",
		job.Handle, job.Name, job.Schedule, job.ScheduleType, job.URL, job.Method, enabled,
		job.NextRun.Format(time.RFC3339), lastRun, job.RunCount)
}

func formatRunLog(log model.RunLog) string {
	return fmt.Sprintf("id=%d job=%s status=%s status_code=%d duration_ms=%d started=%s",
		log.ID, log.JobHandle, log.Status, log.StatusCode, log.Duration, log.StartedAt.Format(time.RFC3339))
}

// executeJob runs a job by making an HTTP request to its URL.
// It retries on failure up to MaxRetries times with a 2-second delay between attempts.
func (h *Handlers) executeJob(job *model.Job) {
	startedAt := time.Now()

	timeout := time.Duration(job.Timeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	client := &http.Client{Timeout: timeout}

	maxRetries := job.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}

	var lastErr string
	var lastStatusCode int
	var totalDuration int64

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(2 * time.Second)
		}

		attemptStart := time.Now()

		var bodyReader io.Reader
		if job.Body != "" {
			bodyReader = strings.NewReader(job.Body)
		}

		req, err := http.NewRequest(job.Method, job.URL, bodyReader)
		if err != nil {
			lastErr = err.Error()
			lastStatusCode = 0
			continue
		}

		// Set custom headers
		for k, v := range job.Headers {
			req.Header.Set(k, v)
		}
		if job.Body != "" && req.Header.Get("Content-Type") == "" {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := client.Do(req)
		attemptDuration := time.Since(attemptStart).Milliseconds()
		totalDuration += attemptDuration

		if err != nil {
			lastErr = err.Error()
			lastStatusCode = 0
			continue
		}

		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		lastStatusCode = resp.StatusCode
		lastErr = ""

		if resp.StatusCode < 400 {
			// Success — record and return
			h.recordRunLog(job, "success", resp.StatusCode, totalDuration, "", startedAt)
			return
		}

		// Non-2xx response — will retry if attempts remain
		lastErr = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}

	// All attempts failed
	h.recordRunLog(job, "failed", lastStatusCode, totalDuration, lastErr, startedAt)
}

func (h *Handlers) recordRunLog(job *model.Job, status string, statusCode int, durationMs int64, errMsg string, startedAt time.Time) {
	now := time.Now()
	log := model.RunLog{
		JobHandle:  job.Handle,
		Workspace:  job.Workspace,
		Status:     status,
		StatusCode: statusCode,
		Duration:   durationMs,
		Error:      errMsg,
		StartedAt:  startedAt,
		FinishedAt: now,
	}

	h.store.AddRunLog(log)

	// Update job stats
	job.LastRun = &now
	job.RunCount++
	nextRun, err := model.CalculateNextRun(job.Schedule, job.ScheduleType, now)
	if err == nil {
		job.NextRun = nextRun
	}
	h.store.UpdateJob(job)
}
