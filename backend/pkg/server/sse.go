package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/venkatvghub/argus/pkg/models"
)

func (s *RESTServer) sseHandler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	jobID := r.URL.Query().Get("jobId")
	if jobID == "" {
		jobID = "*"
	}

	ch, unsubscribe := s.argus.Jobs.Subscribe(jobID)
	defer unsubscribe()

	for {
		select {
		case <-r.Context().Done():
			return
		case job, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(job)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			if job.Status == models.JobStatusCompleted || job.Status == models.JobStatusFailed {
				// Don't return immediately if we want to keep connection open for other jobs
				// but if it's a specific jobID, we can return.
				if jobID != "*" {
					return
				}
			}
		}
	}
}
