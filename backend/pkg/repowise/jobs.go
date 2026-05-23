package repowise

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/venkatvghub/argus/pkg/config"
	"github.com/venkatvghub/argus/pkg/constants"
	"github.com/venkatvghub/argus/pkg/models"
)

type workItem struct {
	jobID string
	fn    func()
}

type JobManager struct {
	mu               sync.RWMutex
	jobs             map[string]*models.Job
	listeners        map[string][]chan models.Job
	cancels          map[string]context.CancelFunc
	workQueue        chan workItem
	listenerBuffer   int
	wg               sync.WaitGroup
	stopMu           sync.RWMutex
	stopped          bool
	stopOnce         sync.Once
	closeCh          chan struct{}
}

const (
	defaultWorkerCount       = 3
	defaultWorkQueueSize     = 32
	defaultJobListenerBuffer = 10
)

func NewJobManager(cfg *config.Config) *JobManager {
	workerCount := defaultWorkerCount
	queueSize := defaultWorkQueueSize
	listenerBuffer := defaultJobListenerBuffer
	if cfg != nil {
		if cfg.WorkerCount > 0 {
			workerCount = cfg.WorkerCount
		}
		if cfg.WorkQueueSize > 0 {
			queueSize = cfg.WorkQueueSize
		}
		if cfg.JobListenerBuffer > 0 {
			listenerBuffer = cfg.JobListenerBuffer
		}
	}

	jm := &JobManager{
		jobs:           make(map[string]*models.Job),
		listeners:      make(map[string][]chan models.Job),
		cancels:        make(map[string]context.CancelFunc),
		workQueue:      make(chan workItem, queueSize),
		listenerBuffer: listenerBuffer,
		closeCh:        make(chan struct{}),
	}
	jm.wg.Add(workerCount)
	for range workerCount {
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
	if l, ok := jm.listeners[constants.AllJobsWildcard]; ok {
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

	ch := make(chan models.Job, jm.listenerBuffer)
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

// Submit enqueues a work function to the bounded worker pool. It blocks until
// queue space is available rather than bypassing the pool with extra goroutines.
func (jm *JobManager) Submit(jobID string, fn func()) {
	item := workItem{jobID: jobID, fn: fn}

	jm.stopMu.RLock()
	if jm.stopped {
		jm.stopMu.RUnlock()
		return
	}
	select {
	case jm.workQueue <- item:
		jm.stopMu.RUnlock()
		return
	default:
		jm.stopMu.RUnlock()
	}

	select {
	case <-jm.closeCh:
		return
	case jm.workQueue <- item:
	}
}

// Close stops accepting new work, drains the queue, and waits for workers to exit.
func (jm *JobManager) Close() {
	jm.stopOnce.Do(func() {
		jm.stopMu.Lock()
		jm.stopped = true
		close(jm.closeCh)
		close(jm.workQueue)
		jm.stopMu.Unlock()
	})
	jm.wg.Wait()
}

func (jm *JobManager) runWorker() {
	defer jm.wg.Done()
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
