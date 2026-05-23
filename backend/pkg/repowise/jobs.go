package repowise

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/venkatvghub/argus/pkg/models"
)

type JobManager struct {
	mu        sync.RWMutex
	jobs      map[string]*models.Job
	listeners map[string][]chan models.Job
	cancels   map[string]context.CancelFunc
}

func NewJobManager() *JobManager {
	return &JobManager{
		jobs:      make(map[string]*models.Job),
		listeners: make(map[string][]chan models.Job),
		cancels:   make(map[string]context.CancelFunc),
	}
}

func (jm *JobManager) CreateJob(jobType string) *models.Job {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	job := &models.Job{
		ID:        uuid.New().String(),
		Type:      jobType,
		Status:    models.JobStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	jm.jobs[job.ID] = job
	return job
}

func (jm *JobManager) UpdateStatus(id string, status models.JobStatus, progress string, err error) {
	jm.mu.Lock()
	job, ok := jm.jobs[id]
	if !ok {
		jm.mu.Unlock()
		return
	}
	job.Status = status
	job.Progress = progress
	if err != nil {
		job.Error = err.Error()
	} else {
		job.Error = ""
	}
	job.UpdatedAt = time.Now()

	// Copy listeners to avoid holding lock while sending
	var jobListeners []chan models.Job
	if l, ok := jm.listeners[id]; ok {
		jobListeners = make([]chan models.Job, len(l))
		copy(jobListeners, l)
	}
	// Global listeners
	if l, ok := jm.listeners["*"]; ok {
		jobListeners = append(jobListeners, l...)
	}
	jm.mu.Unlock()

	for _, ch := range jobListeners {
		select {
		case ch <- *job:
		default:
			// listener too slow, skip or handle
		}
	}
}

func (jm *JobManager) Subscribe(jobID string) chan models.Job {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	ch := make(chan models.Job, 10)
	jm.listeners[jobID] = append(jm.listeners[jobID], ch)
	return ch
}

func (jm *JobManager) GetJob(id string) (*models.Job, bool) {
	jm.mu.RLock()
	defer jm.mu.RUnlock()
	job, ok := jm.jobs[id]
	if !ok {
		return nil, false
	}
	// Return a copy to avoid data races
	jobCopy := *job
	return &jobCopy, true
}

func (jm *JobManager) RegisterCancel(id string, cancel context.CancelFunc) {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	jm.cancels[id] = cancel
}

func (jm *JobManager) CancelJob(id string) error {
	jm.mu.Lock()
	cancel, ok := jm.cancels[id]
	delete(jm.cancels, id)
	jm.mu.Unlock()

	if !ok {
		return fmt.Errorf("job not found or already completed/cancelled")
	}
	cancel()
	jm.UpdateStatus(id, models.JobStatusFailed, "", fmt.Errorf("cancelled"))
	return nil
}

func (jm *JobManager) ListJobs() []models.Job {
	jm.mu.RLock()
	defer jm.mu.RUnlock()
	jobs := make([]models.Job, 0, len(jm.jobs))
	for _, j := range jm.jobs {
		jobs = append(jobs, *j)
	}
	return jobs
}
