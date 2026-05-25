package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/venkatvghub/argus/pkg/argus"
	"github.com/venkatvghub/argus/pkg/constants"
	"github.com/venkatvghub/argus/pkg/logger"
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
		if errors.Is(err, argus.ErrRepoNotFound) {
			http.Error(w, "repo not found", http.StatusNotFound)
			return
		}
		logger.FromContext(r.Context()).Error("get repo symbols failed", "repo_id", repoID, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
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
		writeSSEEvent(w, "[ERROR] "+err.Error())
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
				writeSSEEvent(w, "[ERROR] "+e.Error())
				flusher.Flush()
				return
			}
		case token, ok := <-tokens:
			if !ok {
				writeSSEEvent(w, "[DONE]")
				flusher.Flush()
				return
			}
			writeSSEEvent(w, token)
			flusher.Flush()
		}
	}
}

// postChatMessage handles POST /api/repos/{repoID}/chat/messages.
// It persists the user message, streams the LLM response via SSE, then persists the assistant reply.
func (s *RESTServer) postChatMessage(w http.ResponseWriter, r *http.Request) {
	repoID := strings.TrimSpace(chi.URLParam(r, "repoID"))
	if repoID == "" {
		s.error(w, http.StatusBadRequest, "repoID is required")
		return
	}
	if _, err := s.argus.GetRepository(r.Context(), repoID); err != nil {
		if errors.Is(err, argus.ErrRepoNotFound) {
			s.error(w, http.StatusNotFound, "repo not found")
		} else {
			logger.FromContext(r.Context()).Error("get repository failed", "repo_id", repoID, "error", err)
			s.error(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	var body struct {
		Message        string  `json:"message"`
		ConversationID *string `json:"conversation_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(body.Message) == "" {
		s.error(w, http.StatusBadRequest, "message is required")
		return
	}
	if s.provider == nil {
		s.error(w, http.StatusServiceUnavailable, "LLM provider not configured")
		return
	}

	ctx := r.Context()

	// Resolve or create conversation.
	var convID string
	if body.ConversationID != nil && strings.TrimSpace(*body.ConversationID) != "" {
		convID = strings.TrimSpace(*body.ConversationID)
		conv, err := s.argus.GetConversation(ctx, convID)
		if err != nil {
			if errors.Is(err, argus.ErrConversationNotFound) {
				s.error(w, http.StatusNotFound, err.Error())
			} else {
				logger.FromContext(ctx).Error("get conversation failed", "repo_id", repoID, "conv_id", convID, "error", err)
				s.error(w, http.StatusInternalServerError, "failed to load conversation")
			}
			return
		}
		if conv.RepositoryID != repoID {
			s.error(w, http.StatusNotFound, "conversation not found")
			return
		}
	} else {
		title := body.Message
		if runes := []rune(title); len(runes) > 50 {
			title = string(runes[:50])
		}
		conv, err := s.argus.CreateConversation(ctx, repoID, title)
		if err != nil {
			logger.FromContext(ctx).Error("create conversation failed", "repo_id", repoID, "error", err)
			s.error(w, http.StatusInternalServerError, "failed to create conversation")
			return
		}
		convID = conv.ID
	}

	// Persist the user message before streaming starts.
	userMsg, err := s.argus.CreateChatMessage(ctx, convID, "user", body.Message)
	if err != nil {
		logger.FromContext(ctx).Error("persist user message failed", "conv_id", convID, "error", err)
		s.error(w, http.StatusInternalServerError, "failed to persist message")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.error(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	setSSEHeaders(w, r, s.corsAllowedOrigins())

	tokens, errs, err := s.provider.ChatStream(ctx, repoID, body.Message)
	if err != nil {
		writeSSEEvent(w, "[ERROR] "+err.Error())
		flusher.Flush()
		return
	}

	var sb strings.Builder
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			if e != nil {
				writeSSEEvent(w, "[ERROR] "+e.Error())
				flusher.Flush()
				return
			}
		case token, ok := <-tokens:
			if !ok {
				// Stream done — persist the assistant message.
				assistantMsg, persistErr := s.argus.CreateChatMessage(ctx, convID, "assistant", sb.String())
				if persistErr != nil {
					logger.FromContext(ctx).Error("persist assistant message failed", "conv_id", convID, "error", persistErr)
					writeSSEEvent(w, "[ERROR] failed to persist assistant message")
					flusher.Flush()
					return
				}
				donePayload, _ := json.Marshal(map[string]string{
					"type":            "done",
					"conversation_id": convID,
					"message_id":      assistantMsg.ID,
					"user_message_id": userMsg.ID,
				})
				fmt.Fprintf(w, "data: %s\n\n", donePayload)
				flusher.Flush()
				return
			}
			sb.WriteString(token)
			writeSSEEvent(w, token)
			flusher.Flush()
		}
	}
}

// writeSSEEvent writes a single SSE event, splitting payload on newlines so
// each line is emitted as a separate "data:" field per the EventSource spec.
func writeSSEEvent(w http.ResponseWriter, payload string) {
	for _, line := range strings.Split(payload, "\n") {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	fmt.Fprint(w, "\n")
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
