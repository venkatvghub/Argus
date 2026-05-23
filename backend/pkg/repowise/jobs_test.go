package repowise

import (
	"context"
	"errors"
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
