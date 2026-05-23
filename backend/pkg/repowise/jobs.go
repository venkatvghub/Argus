package repowise

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/venkatvghub/argus/pkg/models"
)

type workItem struct {
	jobID string
	fn    func()
}

type JobManager struct {
	mu        sync.RWMutex
	jobs      map[string]*models.Job
	listeners map[string][]chan models.Job
	cancels   map[string]context.CancelFunc
	workQueue chan workItem
}

const defaultWorkerCount = 3

func NewJobManager() *JobManager {
	jm := &JobManager{
		jobs:      make(map[string]*models.Job),
		listeners: make(map[string][]chan models.Job),
		cancels:   make(map[string]context.CancelFunc),
		workQueue: make(chan workItem, 32),
	}
	for range defaultWorkerCount {
		go jm.runWorker()
	}
	return jm
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
	if status == models.JobStatusCompleted || status == models.JobStatusFailed {
		delete(jm.cancels, id)
	}
	jobSnapshot := *job // snapshot before unlock to avoid race on job struct

	var jobListeners []chan models.Job
	if l, ok := jm.listeners[id]; ok {
		jobListeners = make([]chan models.Job, len(l))
		copy(jobListeners, l)
	}
	if l, ok := jm.listeners["*"]; ok {
		jobListeners = append(jobListeners, l...)
	}
	jm.mu.Unlock()

	for _, ch := range jobListeners {
		select {
		case ch <- jobSnapshot:
		default:
		}
	}
}

func (jm *JobManager) Subscribe(jobID string) (<-chan models.Job, func()) {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	ch := make(chan models.Job, 10)
	jm.listeners[jobID] = append(jm.listeners[jobID], ch)
	unsubscribe := func() {
		jm.mu.Lock()
		defer jm.mu.Unlock()
		listeners := jm.listeners[jobID]
		for i, l := range listeners {
			if l == ch {
				jm.listeners[jobID] = append(listeners[:i], listeners[i+1:]...)
				if len(jm.listeners[jobID]) == 0 {
					delete(jm.listeners, jobID)
				}
				return
			}
		}
	}
	return ch, unsubscribe
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
	if job, exists := jm.jobs[id]; exists {
		if job.Status == models.JobStatusCompleted || job.Status == models.JobStatusFailed {
			jm.mu.Unlock()
			return fmt.Errorf("job already in terminal state: %s", job.Status)
		}
	}
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

// Submit enqueues a work function to the bounded worker pool; returns immediately.
// Falls back to a new goroutine if the queue is full (capacity 32).
func (jm *JobManager) Submit(jobID string, fn func()) {
	item := workItem{jobID: jobID, fn: fn}
	select {
	case jm.workQueue <- item:
	default:
		go jm.executeWork(item)
	}
}

func (jm *JobManager) runWorker() {
	for item := range jm.workQueue {
		jm.executeWork(item)
	}
}

func (jm *JobManager) executeWork(item workItem) {
	defer func() {
		if r := recover(); r != nil {
			jm.UpdateStatus(item.jobID, models.JobStatusFailed, "", fmt.Errorf("panic: %v", r))
		}
	}()
	item.fn()
}
