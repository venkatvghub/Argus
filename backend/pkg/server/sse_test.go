package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/venkatvghub/argus/pkg/models"
	"github.com/venkatvghub/argus/pkg/repowise"
)

func TestSSEHandler(t *testing.T) {
	ctx := context.Background()
	inst, err := repowise.New(ctx, nil)
	assert.NoError(t, err)
	defer inst.Close()

	srv := NewRESTServer(inst)

	t.Run("Subscribe and Receive Updates", func(t *testing.T) {
		job := inst.Jobs.CreateJob("sse_test")

		req := httptest.NewRequest("GET", "/api/events?jobId="+job.ID, nil)
		rr := httptest.NewRecorder()

		// Since sseHandler is blocking, we run it in a goroutine
		ctx, cancel := context.WithTimeout(req.Context(), 1*time.Second)
		defer cancel()
		req = req.WithContext(ctx)

		done := make(chan bool)
		go func() {
			srv.sseHandler(rr, req)
			done <- true
		}()

		// Give it a moment to start subscribing
		time.Sleep(100 * time.Millisecond)

		// Send an update
		inst.Jobs.UpdateStatus(job.ID, models.JobStatusInProgress, "50%", nil)
		inst.Jobs.UpdateStatus(job.ID, models.JobStatusCompleted, "100%", nil)

		select {
		case <-done:
			// Handler returned as expected on completion
		case <-time.After(500 * time.Millisecond):
			// If it hasn't returned, that's also fine if it's still waiting for context
		}

		body := rr.Body.String()
		assert.Contains(t, body, "data: {")
		assert.Contains(t, body, "in_progress")
		assert.Contains(t, body, "completed")
		assert.Contains(t, body, "50%")
		assert.Contains(t, body, "100%")
	})

	t.Run("Global Subscription", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/events", nil) // no jobId
		rr := httptest.NewRecorder()

		ctx, cancel := context.WithTimeout(req.Context(), 500*time.Millisecond)
		defer cancel()
		req = req.WithContext(ctx)

		go func() {
			srv.sseHandler(rr, req)
		}()

		time.Sleep(100 * time.Millisecond)

		job := inst.Jobs.CreateJob("global_test")
		inst.Jobs.UpdateStatus(job.ID, models.JobStatusCompleted, "Done", nil)

		time.Sleep(100 * time.Millisecond)

		body := rr.Body.String()
		assert.Contains(t, body, "global_test")
	})
}

func TestSSEParsing(t *testing.T) {
	// More detailed parsing test
	ctx := context.Background()
	inst, err := repowise.New(ctx, nil)
	if err != nil {
		t.Fatalf("failed to create repowise instance: %v", err)
	}
	defer inst.Close()
	srv := NewRESTServer(inst)

	job := inst.Jobs.CreateJob("parse_test")

	req := httptest.NewRequest("GET", "/api/events?jobId="+job.ID, nil)
	rr := httptest.NewRecorder()

	// Create a pipe to read the stream
	pr, pw := httptest.NewRecorder().Body, httptest.NewRecorder().Body // This won't work as expected with Recorder
	_ = pr
	_ = pw

	// We'll use a custom response writer that we can read from
	type pipeWriter struct {
		http.ResponseWriter
		ch chan string
	}
	// Actually httptest.NewRecorder is fine if we can read its body periodically.
	// But it only exposes body as a buffer.

	// Let's just use the body string and parse it as SSE
	go func() {
		time.Sleep(50 * time.Millisecond)
		inst.Jobs.UpdateStatus(job.ID, models.JobStatusCompleted, "100%", nil)
	}()

	srv.sseHandler(rr, req)

	scanner := bufio.NewScanner(strings.NewReader(rr.Body.String()))
	found := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			var j models.Job
			err := json.Unmarshal([]byte(data), &j)
			assert.NoError(t, err)
			if j.ID == job.ID && j.Status == models.JobStatusCompleted {
				found = true
			}
		}
	}
	assert.True(t, found)
}
