package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/venkatvghub/argus/pkg/constants"
	"github.com/venkatvghub/argus/pkg/models"
)

func (s *RESTServer) chatStreamHandler(w http.ResponseWriter, r *http.Request) {
	repoID := strings.TrimSpace(r.URL.Query().Get("repoID"))
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if repoID == "" {
		http.Error(w, "repoID is required", http.StatusBadRequest)
		return
	}
	if query == "" {
		http.Error(w, "q is required", http.StatusBadRequest)
		return
	}
	if s.provider == nil {
		http.Error(w, "LLM provider not configured", http.StatusServiceUnavailable)
		return
	}
	if _, err := s.argus.GetRepoSymbols(r.Context(), repoID); err != nil {
		http.Error(w, "repo not found", http.StatusNotFound)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	setSSEHeaders(w, r, s.corsAllowedOrigins())

	tokens, errs, err := s.provider.ChatStream(r.Context(), repoID, query)
	if err != nil {
		fmt.Fprintf(w, "data: [ERROR] %s\n\n", err.Error())
		flusher.Flush()
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case e, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			if e != nil {
				fmt.Fprintf(w, "data: [ERROR] %s\n\n", e.Error())
				flusher.Flush()
				return
			}
		case token, ok := <-tokens:
			if !ok {
				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", token)
			flusher.Flush()
		}
	}
}

func (s *RESTServer) sseHandler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	setSSEHeaders(w, r, s.corsAllowedOrigins())

	jobID := r.URL.Query().Get("jobId")
	if jobID == "" {
		jobID = constants.AllJobsWildcard
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
				if jobID != constants.AllJobsWildcard {
					return
				}
			}
		}
	}
}
