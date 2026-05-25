package persistence_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/venkatvghub/argus/pkg/models"
)

// TestCreateJob tests Job creation and retrieval.
func TestCreateJob(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	upsertTestRepo(t, db, "repo-001")

	job := models.Job{
		ID:       "job-001",
		RepoID:   "repo-001",
		Type:     "analysis",
		Status:   models.JobStatusPending,
		Progress: "0%",
		Error:    "",
	}

	err := db.CreateJob(ctx, job)
	require.NoError(t, err)

	// Verify job was created with correct values
	retrieved, err := db.GetJob(ctx, "job-001")
	require.NoError(t, err)

	assert.Equal(t, job.ID, retrieved.ID)
	assert.Equal(t, job.RepoID, retrieved.RepoID)
	assert.Equal(t, job.Type, retrieved.Type)
	assert.Equal(t, job.Status, retrieved.Status)
	assert.Equal(t, job.Progress, retrieved.Progress)
	assert.NotZero(t, retrieved.CreatedAt)
	assert.NotZero(t, retrieved.UpdatedAt)
}

// TestGetJob tests retrieving a job by ID and error when not found.
func TestGetJob(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	t.Run("job exists", func(t *testing.T) {
		upsertTestRepo(t, db, "repo-test")
		job := models.Job{
			ID:     "job-test",
			RepoID: "repo-test",
			Type:   "analysis",
			Status: models.JobStatusInProgress,
		}
		err := db.CreateJob(ctx, job)
		require.NoError(t, err)

		retrieved, err := db.GetJob(ctx, "job-test")
		require.NoError(t, err)
		assert.Equal(t, job.ID, retrieved.ID)
		assert.Equal(t, models.JobStatusInProgress, retrieved.Status)
	})

	t.Run("job not found", func(t *testing.T) {
		_, err := db.GetJob(ctx, "nonexistent-job")
		assert.Error(t, err)
	})
}

// TestListJobs tests listing jobs filtered by repo ID and all repos.
func TestListJobs(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	upsertTestRepo(t, db, "repo-1")
	upsertTestRepo(t, db, "repo-2")

	// Create jobs for different repos
	jobs := []models.Job{
		{ID: "job-1", RepoID: "repo-1", Type: "analysis", Status: models.JobStatusPending},
		{ID: "job-2", RepoID: "repo-1", Type: "analysis", Status: models.JobStatusInProgress},
		{ID: "job-3", RepoID: "repo-2", Type: "analysis", Status: models.JobStatusCompleted},
		{ID: "job-4", RepoID: "repo-2", Type: "analysis", Status: models.JobStatusFailed},
	}

	for _, job := range jobs {
		err := db.CreateJob(ctx, job)
		require.NoError(t, err)
	}

	t.Run("list jobs for specific repo", func(t *testing.T) {
		result, err := db.ListJobs(ctx, "repo-1")
		require.NoError(t, err)
		require.Len(t, result, 2)
		// Verify all are from repo-1 and in descending order by created_at
		jobIDSet := make(map[string]bool)
		for _, job := range result {
			assert.Equal(t, "repo-1", job.RepoID)
			jobIDSet[job.ID] = true
		}
		expectedIDs := map[string]bool{
			"job-1": true,
			"job-2": true,
		}
		assert.Equal(t, expectedIDs, jobIDSet)
		// Verify descending order
		assert.True(t, result[0].CreatedAt.After(result[1].CreatedAt) || result[0].CreatedAt.Equal(result[1].CreatedAt))
	})

	t.Run("list all jobs when repoID empty", func(t *testing.T) {
		result, err := db.ListJobs(ctx, "")
		require.NoError(t, err)
		require.Len(t, result, 4)
		// Verify all job IDs are present and in descending order by created_at
		jobIDSet := make(map[string]bool)
		for _, job := range result {
			jobIDSet[job.ID] = true
		}
		expectedIDs := map[string]bool{
			"job-1": true,
			"job-2": true,
			"job-3": true,
			"job-4": true,
		}
		assert.Equal(t, expectedIDs, jobIDSet)
		// Verify that each job in the result is newer than or equal to the next one
		for i := 0; i < len(result)-1; i++ {
			assert.True(t, result[i].CreatedAt.After(result[i+1].CreatedAt) || result[i].CreatedAt.Equal(result[i+1].CreatedAt),
				"jobs should be in descending order by created_at")
		}
	})

	t.Run("list jobs for non-existent repo returns empty", func(t *testing.T) {
		result, err := db.ListJobs(ctx, "repo-nonexistent")
		require.NoError(t, err)
		assert.Empty(t, result)
	})
}

// TestUpdateJobStatus tests updating job status, progress, and error.
func TestUpdateJobStatus(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	upsertTestRepo(t, db, "repo-1")

	job := models.Job{
		ID:       "job-update",
		RepoID:   "repo-1",
		Type:     "analysis",
		Status:   models.JobStatusPending,
		Progress: "0%",
	}
	err := db.CreateJob(ctx, job)
	require.NoError(t, err)

	t.Run("update status to in_progress", func(t *testing.T) {
		err := db.UpdateJobStatus(ctx, "job-update", string(models.JobStatusInProgress), "50%", "")
		require.NoError(t, err)

		updated, err := db.GetJob(ctx, "job-update")
		require.NoError(t, err)
		assert.Equal(t, models.JobStatusInProgress, updated.Status)
		assert.Equal(t, "50%", updated.Progress)
		assert.Empty(t, updated.Error)
	})

	t.Run("update status to failed with error message", func(t *testing.T) {
		err := db.UpdateJobStatus(ctx, "job-update", string(models.JobStatusFailed), "50%", "analysis timeout")
		require.NoError(t, err)

		updated, err := db.GetJob(ctx, "job-update")
		require.NoError(t, err)
		assert.Equal(t, models.JobStatusFailed, updated.Status)
		assert.Equal(t, "analysis timeout", updated.Error)
	})

	t.Run("update non-existent job succeeds silently", func(t *testing.T) {
		err := db.UpdateJobStatus(ctx, "nonexistent", string(models.JobStatusCompleted), "100%", "")
		require.NoError(t, err) // SQLite does not error on 0 affected rows
	})
}

// TestCreateConversation tests conversation creation.
func TestCreateConversation(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	upsertTestRepo(t, db, "repo-001")

	conv := models.Conversation{
		ID:           "conv-001",
		RepositoryID: "repo-001",
		Title:        "Initial Analysis",
		MessageCount: 0,
	}

	err := db.CreateConversation(ctx, conv)
	require.NoError(t, err)

	// Verify conversation was created
	retrieved, err := db.GetConversation(ctx, "conv-001")
	require.NoError(t, err)

	assert.Equal(t, conv.ID, retrieved.ID)
	assert.Equal(t, conv.RepositoryID, retrieved.RepositoryID)
	assert.Equal(t, conv.Title, retrieved.Title)
	assert.Equal(t, 0, retrieved.MessageCount) // Created with 0 message count
	assert.NotZero(t, retrieved.CreatedAt)
	assert.NotZero(t, retrieved.UpdatedAt)
}

// TestGetConversation tests retrieving a conversation by ID.
func TestGetConversation(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	upsertTestRepo(t, db, "repo-1")

	conv := models.Conversation{
		ID:           "conv-test",
		RepositoryID: "repo-1",
		Title:        "Test Conversation",
	}
	err := db.CreateConversation(ctx, conv)
	require.NoError(t, err)

	t.Run("conversation exists", func(t *testing.T) {
		retrieved, err := db.GetConversation(ctx, "conv-test")
		require.NoError(t, err)
		assert.Equal(t, conv.Title, retrieved.Title)
		assert.Equal(t, conv.RepositoryID, retrieved.RepositoryID)
	})

	t.Run("conversation not found", func(t *testing.T) {
		_, err := db.GetConversation(ctx, "nonexistent-conv")
		assert.Error(t, err)
	})
}

// TestListConversations tests listing conversations for a repository.
func TestListConversations(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	upsertTestRepo(t, db, "repo-1")
	upsertTestRepo(t, db, "repo-2")

	// Create conversations for different repos
	convs := []models.Conversation{
		{ID: "conv-1", RepositoryID: "repo-1", Title: "Conv 1"},
		{ID: "conv-2", RepositoryID: "repo-1", Title: "Conv 2"},
		{ID: "conv-3", RepositoryID: "repo-2", Title: "Conv 3"},
	}

	for _, conv := range convs {
		err := db.CreateConversation(ctx, conv)
		require.NoError(t, err)
	}

	t.Run("list conversations for specific repo", func(t *testing.T) {
		result, err := db.ListConversations(ctx, "repo-1")
		require.NoError(t, err)
		require.Len(t, result, 2)
		// Verify all are for repo-1
		for _, conv := range result {
			assert.Equal(t, "repo-1", conv.RepositoryID)
		}
	})

	t.Run("list conversations for non-existent repo returns empty", func(t *testing.T) {
		result, err := db.ListConversations(ctx, "repo-nonexistent")
		require.NoError(t, err)
		assert.Empty(t, result)
	})
}

// TestDeleteConversation tests deleting a conversation.
func TestDeleteConversation(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	upsertTestRepo(t, db, "repo-1")

	conv := models.Conversation{
		ID:           "conv-delete",
		RepositoryID: "repo-1",
		Title:        "To Delete",
	}
	err := db.CreateConversation(ctx, conv)
	require.NoError(t, err)

	// Delete the conversation
	err = db.DeleteConversation(ctx, "conv-delete")
	require.NoError(t, err)

	// Verify it's gone
	_, err = db.GetConversation(ctx, "conv-delete")
	assert.Error(t, err)
}

// TestIncrementMessageCount tests incrementing the message count.
func TestIncrementMessageCount(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	upsertTestRepo(t, db, "repo-1")

	conv := models.Conversation{
		ID:           "conv-count",
		RepositoryID: "repo-1",
		Title:        "Message Count Test",
		MessageCount: 0,
	}
	err := db.CreateConversation(ctx, conv)
	require.NoError(t, err)

	// Increment multiple times
	for i := 1; i <= 3; i++ {
		err := db.IncrementMessageCount(ctx, "conv-count")
		require.NoError(t, err)

		conv, err := db.GetConversation(ctx, "conv-count")
		require.NoError(t, err)
		assert.Equal(t, i, conv.MessageCount)
	}
}

// TestCreateChatMessage tests creating a chat message.
func TestCreateChatMessage(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	upsertTestRepo(t, db, "repo-1")

	// Create conversation first
	conv := models.Conversation{
		ID:           "conv-msg",
		RepositoryID: "repo-1",
		Title:        "Message Test",
	}
	err := db.CreateConversation(ctx, conv)
	require.NoError(t, err)

	msg := models.ChatMessage{
		ID:             "msg-001",
		ConversationID: "conv-msg",
		Role:           "user",
		Content:        "What is this repo?",
	}

	err = db.CreateChatMessage(ctx, msg)
	require.NoError(t, err)

	// Verify message was created
	messages, err := db.ListChatMessages(ctx, "conv-msg")
	require.NoError(t, err)
	require.Len(t, messages, 1)

	assert.Equal(t, msg.ID, messages[0].ID)
	assert.Equal(t, msg.Role, messages[0].Role)
	assert.Equal(t, msg.Content, messages[0].Content)
	assert.NotZero(t, messages[0].CreatedAt)
}

// TestListChatMessages tests listing messages for a conversation.
func TestListChatMessages(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	upsertTestRepo(t, db, "repo-1")

	// Create conversation
	conv := models.Conversation{
		ID:           "conv-msgs",
		RepositoryID: "repo-1",
		Title:        "Messages Test",
	}
	err := db.CreateConversation(ctx, conv)
	require.NoError(t, err)

	// Create multiple messages
	messages := []models.ChatMessage{
		{ID: "msg-1", ConversationID: "conv-msgs", Role: "user", Content: "Hello"},
		{ID: "msg-2", ConversationID: "conv-msgs", Role: "assistant", Content: "Hi there"},
		{ID: "msg-3", ConversationID: "conv-msgs", Role: "user", Content: "How are you?"},
	}

	for _, msg := range messages {
		err := db.CreateChatMessage(ctx, msg)
		require.NoError(t, err)
	}

	t.Run("list messages in chronological order", func(t *testing.T) {
		result, err := db.ListChatMessages(ctx, "conv-msgs")
		require.NoError(t, err)
		require.Len(t, result, 3)

		// Verify order (created_at ASC)
		assert.Equal(t, "msg-1", result[0].ID)
		assert.Equal(t, "msg-2", result[1].ID)
		assert.Equal(t, "msg-3", result[2].ID)

		// Verify roles
		assert.Equal(t, "user", result[0].Role)
		assert.Equal(t, "assistant", result[1].Role)
		assert.Equal(t, "user", result[2].Role)
	})

	t.Run("list messages for non-existent conversation returns empty", func(t *testing.T) {
		result, err := db.ListChatMessages(ctx, "conv-nonexistent")
		require.NoError(t, err)
		assert.Empty(t, result)
	})
}

// TestChatMessageDeleteConversation tests that deleting a conversation with messages succeeds.
// Note: Cascade delete of messages requires foreign key enforcement which may not be enabled.
// This test verifies the delete operation succeeds without errors.
func TestChatMessageDeleteConversation(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	upsertTestRepo(t, db, "repo-1")

	// Create conversation with messages
	conv := models.Conversation{
		ID:           "conv-delete-test",
		RepositoryID: "repo-1",
		Title:        "Delete Test",
	}
	err := db.CreateConversation(ctx, conv)
	require.NoError(t, err)

	msg := models.ChatMessage{
		ID:             "msg-delete-test",
		ConversationID: "conv-delete-test",
		Role:           "user",
		Content:        "Test message",
	}
	err = db.CreateChatMessage(ctx, msg)
	require.NoError(t, err)

	// Delete conversation succeeds
	err = db.DeleteConversation(ctx, "conv-delete-test")
	require.NoError(t, err)

	// Verify conversation is gone
	_, err = db.GetConversation(ctx, "conv-delete-test")
	assert.Error(t, err)
}
