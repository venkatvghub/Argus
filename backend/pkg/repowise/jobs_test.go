package repowise

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/venkatvghub/argus/pkg/models"
)

func TestJobManager(t *testing.T) {
	jm := NewJobManager()

	t.Run("Create and Get Job", func(t *testing.T) {
		job := jm.CreateJob("test")
		assert.NotEmpty(t, job.ID)
		assert.Equal(t, "test", job.Type)
		assert.Equal(t, models.JobStatusPending, job.Status)

		fetched, ok := jm.GetJob(job.ID)
		assert.True(t, ok)
		assert.Equal(t, job.ID, fetched.ID)
	})

	t.Run("Update Status and Notify", func(t *testing.T) {
		job := jm.CreateJob("notify_test")
		ch, _ := jm.Subscribe(job.ID)

		go func() {
			jm.UpdateStatus(job.ID, models.JobStatusInProgress, "50%", nil)
			jm.UpdateStatus(job.ID, models.JobStatusCompleted, "100%", nil)
		}()

		var update1 models.Job
		select {
		case update1 = <-ch:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for update1")
		}
		assert.Equal(t, models.JobStatusInProgress, update1.Status)
		assert.Equal(t, "50%", update1.Progress)

		var update2 models.Job
		select {
		case update2 = <-ch:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for update2")
		}
		assert.Equal(t, models.JobStatusCompleted, update2.Status)
		assert.Equal(t, "100%", update2.Progress)
	})

	t.Run("Job Cancellation", func(t *testing.T) {
		job := jm.CreateJob("cancel_test")
		ctx, cancel := context.WithCancel(context.Background())
		jm.RegisterCancel(job.ID, cancel)

		err := jm.CancelJob(job.ID)
		assert.NoError(t, err)

		select {
		case <-ctx.Done():
			// Success
		case <-time.After(100 * time.Millisecond):
			t.Fatal("context was not cancelled")
		}

		fetched, _ := jm.GetJob(job.ID)
		assert.Equal(t, models.JobStatusFailed, fetched.Status)
		assert.Equal(t, "cancelled", fetched.Error)
	})

	t.Run("Error Handling", func(t *testing.T) {
		job := jm.CreateJob("error_test")
		testErr := errors.New("something went wrong")
		jm.UpdateStatus(job.ID, models.JobStatusFailed, "0%", testErr)

		fetched, _ := jm.GetJob(job.ID)
		assert.Equal(t, models.JobStatusFailed, fetched.Status)
		assert.Equal(t, testErr.Error(), fetched.Error)
	})
}

func TestBackgroundExecution(t *testing.T) {
	// This tests that Analyze indeed runs in background and returns a job ID
	ctx := context.Background()
	inst, err := New(ctx, nil)
	assert.NoError(t, err)
	defer inst.Close()

	// Use a fake repo path for testing
	jobID, err := inst.Analyze(ctx, ".")
	assert.NoError(t, err)
	assert.NotEmpty(t, jobID)

	// Job should be created and likely in progress or pending
	job, ok := inst.Jobs.GetJob(jobID)
	assert.True(t, ok)
	assert.Contains(t, []models.JobStatus{models.JobStatusPending, models.JobStatusInProgress, models.JobStatusCompleted}, job.Status)
}

func TestWorkerPoolSubmit(t *testing.T) {
	jm := NewJobManager()
	job := jm.CreateJob("pool_submit_test")

	executed := make(chan struct{}, 1)
	fn := func() {
		close(executed)
		jm.UpdateStatus(job.ID, models.JobStatusCompleted, "100%", nil)
	}

	ch, unsubscribe := jm.Subscribe(job.ID)
	defer unsubscribe()

	jm.Submit(job.ID, fn)

	select {
	case <-executed:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: fn never executed")
	}

	var finalJob models.Job
	select {
	case finalJob = <-ch:
		assert.Equal(t, models.JobStatusCompleted, finalJob.Status)
		assert.Equal(t, "100%", finalJob.Progress)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for job completion")
	}
}

func TestWorkerPoolPanicRecovery(t *testing.T) {
	// Test that panics in fn are recovered and job transitions to Failed
	jm := NewJobManager()
	job := jm.CreateJob("pool_panic_test")

	panicFunc := func() {
		panic("test panic")
	}

	ch, unsubscribe := jm.Subscribe(job.ID)
	defer unsubscribe()

	// Submit work that will panic
	jm.Submit(job.ID, panicFunc)

	// Wait for completion
	var finalJob models.Job
	select {
	case finalJob = <-ch:
		assert.Equal(t, models.JobStatusFailed, finalJob.Status)
		assert.True(t, strings.Contains(finalJob.Error, "panic"), "error should contain 'panic'")
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for job completion")
	}

	// Verify job in manager has failed status
	fetched, ok := jm.GetJob(job.ID)
	assert.True(t, ok)
	assert.Equal(t, models.JobStatusFailed, fetched.Status)
	assert.True(t, strings.Contains(fetched.Error, "panic"), "stored error should contain 'panic'")
}

func TestWorkerPoolConcurrency(t *testing.T) {
	// Test that all 6 jobs (2x pool size) complete successfully
	jm := NewJobManager()
	jobCount := 6
	completionChannel := make(chan string, jobCount)

	jobs := make([]*models.Job, jobCount)
	for i := 0; i < jobCount; i++ {
		jobs[i] = jm.CreateJob(fmt.Sprintf("concurrent_job_%d", i))
	}

	// Submit all jobs
	for i := 0; i < jobCount; i++ {
		idx := i // capture loop variable
		jm.Submit(jobs[idx].ID, func() {
			time.Sleep(10 * time.Millisecond) // simulate some work
			completionChannel <- jobs[idx].ID
		})
	}

	// Wait for all jobs to complete by collecting from global subscription
	globalCh, unsubscribe := jm.Subscribe("*")
	defer unsubscribe()

	completedJobs := make(map[string]bool)
	deadline := time.After(3 * time.Second)

	// Collect completion events from worker execution
	for i := 0; i < jobCount; i++ {
		select {
		case jobID := <-completionChannel:
			completedJobs[jobID] = true
			// Mark as completed in job manager
			jm.UpdateStatus(jobID, models.JobStatusCompleted, "100%", nil)
		case <-deadline:
			t.Fatalf("timeout waiting for all jobs to complete; got %d/%d", len(completedJobs), jobCount)
		}
	}

	// Verify all jobs completed
	for i := 0; i < jobCount; i++ {
		assert.True(t, completedJobs[jobs[i].ID], "job %s should have completed", jobs[i].ID)
	}

	// Verify via Subscribe channel
	completedViaSubscribe := 0
	deadline = time.After(500 * time.Millisecond)
	for {
		select {
		case job := <-globalCh:
			if job.Status == models.JobStatusCompleted {
				completedViaSubscribe++
			}
			if completedViaSubscribe >= jobCount {
				assert.Equal(t, jobCount, completedViaSubscribe)
				return
			}
		case <-deadline:
			assert.GreaterOrEqual(t, completedViaSubscribe, jobCount, "should have received %d completed jobs via subscribe", jobCount)
			return
		}
	}
}
