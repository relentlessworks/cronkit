package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/relentlessworks/cronkit/internal/model"
)

// Store manages all data with JSON file persistence.
type Store struct {
	mu       sync.RWMutex
	filePath string
	data     *storeData
}

type storeData struct {
	Workspaces map[string]*model.Workspace `json:"workspaces"`
	Jobs       map[string]*model.Job       `json:"jobs"`
	RunLogs    []model.RunLog              `json:"run_logs"`
	Tokens     map[string]*model.Token     `json:"tokens"`
	OTPs       map[string]*model.OTP       `json:"otps"`
}

// New creates a new store with the given file path.
func New(filePath string) (*Store, error) {
	s := &Store{
		filePath: filePath,
		data: &storeData{
			Workspaces: make(map[string]*model.Workspace),
			Jobs:       make(map[string]*model.Job),
			Tokens:     make(map[string]*model.Token),
			OTPs:       make(map[string]*model.OTP),
		},
	}

	if err := s.load(); err != nil {
		return nil, fmt.Errorf("failed to load store: %w", err)
	}

	return s, nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // fresh start
		}
		return err
	}

	var d storeData
	if err := json.Unmarshal(data, &d); err != nil {
		return err
	}

	if d.Workspaces == nil {
		d.Workspaces = make(map[string]*model.Workspace)
	}
	if d.Jobs == nil {
		d.Jobs = make(map[string]*model.Job)
	}
	if d.Tokens == nil {
		d.Tokens = make(map[string]*model.Token)
	}
	if d.OTPs == nil {
		d.OTPs = make(map[string]*model.OTP)
	}

	s.data = &d
	return nil
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.filePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return os.WriteFile(s.filePath, data, 0644)
}

// --- Workspace operations ---

func (s *Store) CreateWorkspace(ws *model.Workspace) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Workspaces[ws.Handle] = ws
	return s.save()
}

func (s *Store) GetWorkspace(handle string) (*model.Workspace, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ws, ok := s.data.Workspaces[handle]
	return ws, ok
}

func (s *Store) GetWorkspaceByName(name string) (*model.Workspace, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, ws := range s.data.Workspaces {
		if ws.Name == name {
			return ws, true
		}
	}
	return nil, false
}

// --- Job operations ---

func (s *Store) CreateJob(job *model.Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Jobs[job.Handle] = job
	return s.save()
}

func (s *Store) GetJob(handle string) (*model.Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.data.Jobs[handle]
	return job, ok
}

func (s *Store) UpdateJob(job *model.Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job.UpdatedAt = time.Now()
	s.data.Jobs[job.Handle] = job
	return s.save()
}

func (s *Store) DeleteJob(handle string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data.Jobs, handle)
	return s.save()
}

func (s *Store) ListJobs(workspace string) []*model.Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var jobs []*model.Job
	for _, job := range s.data.Jobs {
		if job.Workspace == workspace {
			jobs = append(jobs, job)
		}
	}
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].CreatedAt.Before(jobs[j].CreatedAt)
	})
	return jobs
}

func (s *Store) ListAllJobs() []*model.Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var jobs []*model.Job
	for _, job := range s.data.Jobs {
		jobs = append(jobs, job)
	}
	return jobs
}

// --- Run log operations ---

func (s *Store) AddRunLog(log model.RunLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	log.ID = int64(len(s.data.RunLogs) + 1)
	s.data.RunLogs = append(s.data.RunLogs, log)
	// Keep only last 1000 logs
	if len(s.data.RunLogs) > 1000 {
		s.data.RunLogs = s.data.RunLogs[len(s.data.RunLogs)-1000:]
	}
	return s.save()
}

func (s *Store) ListRunLogs(workspace, jobHandle string, limit int) []model.RunLog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var logs []model.RunLog
	for i := len(s.data.RunLogs) - 1; i >= 0; i-- {
		log := s.data.RunLogs[i]
		if log.Workspace != workspace {
			continue
		}
		if jobHandle != "" && log.JobHandle != jobHandle {
			continue
		}
		logs = append(logs, log)
		if limit > 0 && len(logs) >= limit {
			break
		}
	}
	return logs
}

// --- Token operations ---

func (s *Store) SaveToken(token *model.Token) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Tokens[token.Token] = token
	return s.save()
}

func (s *Store) GetToken(token string) (*model.Token, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.data.Tokens[token]
	return t, ok
}

func (s *Store) DeleteToken(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data.Tokens, token)
	return s.save()
}

// --- OTP operations ---

func (s *Store) SaveOTP(otp *model.OTP) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.OTPs[otp.Email] = otp
	return s.save()
}

func (s *Store) GetOTP(email string) (*model.OTP, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	otp, ok := s.data.OTPs[email]
	return otp, ok
}

func (s *Store) DeleteOTP(email string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data.OTPs, email)
	return s.save()
}
