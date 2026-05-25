package argus_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/venkatvghub/argus/pkg/argus"
	"github.com/venkatvghub/argus/pkg/config"
)

func testDBURLExternal(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("ARGUS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ARGUS_TEST_DATABASE_URL not set")
	}
	return dsn
}

func setupTestInstance(t *testing.T) (*argus.Instance, func()) {
	t.Helper()
	cfg := &config.Config{
		DatabaseURL: testDBURLExternal(t),
		LogLevel:    "error",
		AppName:     "ArgusTest",
	}

	ctx := context.Background()
	instance, err := argus.New(ctx, cfg)
	require.NoError(t, err)

	return instance, func() {
		instance.Close()
	}
}

// TestCreateConversation tests creating a conversation via Instance.
func TestCreateConversation(t *testing.T) {
	instance, cleanup := setupTestInstance(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("create conversation with valid params", func(t *testing.T) {
		conv, err := instance.CreateConversation(ctx, "repo-001", "Analyze Code Quality")
		require.NoError(t, err)

		assert.NotEmpty(t, conv.ID)
		assert.Equal(t, "repo-001", conv.RepositoryID)
		assert.Equal(t, "Analyze Code Quality", conv.Title)

		// Retrieve to verify timestamps were set
		retrieved, err := instance.GetConversation(ctx, conv.ID)
		require.NoError(t, err)
		assert.Equal(t, 0, retrieved.MessageCount)
		assert.NotZero(t, retrieved.CreatedAt)
		assert.NotZero(t, retrieved.UpdatedAt)
	})

	t.Run("multiple conversations are unique", func(t *testing.T) {
		conv1, err := instance.CreateConversation(ctx, "repo-001", "Title 1")
		require.NoError(t, err)

		conv2, err := instance.CreateConversation(ctx, "repo-001", "Title 2")
		require.NoError(t, err)

		assert.NotEqual(t, conv1.ID, conv2.ID)
	})
}

// TestGetConversation tests retrieving a conversation via Instance.
func TestGetConversation(t *testing.T) {
	instance, cleanup := setupTestInstance(t)
	defer cleanup()

	ctx := context.Background()

	conv, err := instance.CreateConversation(ctx, "repo-001", "Test Conv")
	require.NoError(t, err)

	t.Run("retrieve existing conversation", func(t *testing.T) {
		retrieved, err := instance.GetConversation(ctx, conv.ID)
		require.NoError(t, err)

		assert.Equal(t, conv.ID, retrieved.ID)
		assert.Equal(t, "repo-001", retrieved.RepositoryID)
		assert.Equal(t, "Test Conv", retrieved.Title)
	})

	t.Run("get non-existent conversation returns error", func(t *testing.T) {
		_, err := instance.GetConversation(ctx, "nonexistent-conv")
		assert.Error(t, err)
	})
}

// TestListConversations tests listing conversations via Instance.
func TestListConversations(t *testing.T) {
	instance, cleanup := setupTestInstance(t)
	defer cleanup()

	ctx := context.Background()

	// Create conversations for different repos
	conv1, err := instance.CreateConversation(ctx, "repo-1", "Conv 1")
	require.NoError(t, err)

	conv2, err := instance.CreateConversation(ctx, "repo-1", "Conv 2")
	require.NoError(t, err)

	conv3, err := instance.CreateConversation(ctx, "repo-2", "Conv 3")
	require.NoError(t, err)

	t.Run("list conversations for repo", func(t *testing.T) {
		result, err := instance.ListConversations(ctx, "repo-1")
		require.NoError(t, err)
		require.Len(t, result, 2)

		ids := map[string]bool{
			conv1.ID: true,
			conv2.ID: true,
		}
		for _, conv := range result {
			assert.True(t, ids[conv.ID], "unexpected conversation ID: %s", conv.ID)
			assert.Equal(t, "repo-1", conv.RepositoryID)
		}
	})

	t.Run("list conversations for different repo", func(t *testing.T) {
		result, err := instance.ListConversations(ctx, "repo-2")
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, conv3.ID, result[0].ID)
	})

	t.Run("list conversations for non-existent repo returns empty", func(t *testing.T) {
		result, err := instance.ListConversations(ctx, "repo-nonexistent")
		require.NoError(t, err)
		assert.Empty(t, result)
	})
}

// TestDeleteConversation tests deleting a conversation via Instance.
func TestDeleteConversation(t *testing.T) {
	instance, cleanup := setupTestInstance(t)
	defer cleanup()

	ctx := context.Background()

	conv, err := instance.CreateConversation(ctx, "repo-001", "To Delete")
	require.NoError(t, err)

	// Delete the conversation
	err = instance.DeleteConversation(ctx, conv.ID)
	require.NoError(t, err)

	// Verify it's gone
	_, err = instance.GetConversation(ctx, conv.ID)
	assert.Error(t, err)
}

// TestCreateChatMessage tests creating a chat message via Instance.
func TestCreateChatMessage(t *testing.T) {
	instance, cleanup := setupTestInstance(t)
	defer cleanup()

	ctx := context.Background()

	conv, err := instance.CreateConversation(ctx, "repo-001", "Chat Test")
	require.NoError(t, err)

	t.Run("create user message", func(t *testing.T) {
		msg, err := instance.CreateChatMessage(ctx, conv.ID, "user", "What's this repo?")
		require.NoError(t, err)

		assert.NotEmpty(t, msg.ID)
		assert.Equal(t, conv.ID, msg.ConversationID)
		assert.Equal(t, "user", msg.Role)
		assert.Equal(t, "What's this repo?", msg.Content)

		// Verify timestamp was set in database
		messages, err := instance.ListChatMessages(ctx, conv.ID)
		require.NoError(t, err)
		require.NotEmpty(t, messages)
		assert.NotZero(t, messages[0].CreatedAt)
	})

	t.Run("create assistant message", func(t *testing.T) {
		msg, err := instance.CreateChatMessage(ctx, conv.ID, "assistant", "This is a test repo.")
		require.NoError(t, err)

		assert.Equal(t, "assistant", msg.Role)
		assert.Equal(t, "This is a test repo.", msg.Content)
	})

	t.Run("messages are created with unique IDs", func(t *testing.T) {
		msg1, err := instance.CreateChatMessage(ctx, conv.ID, "user", "Msg 1")
		require.NoError(t, err)

		msg2, err := instance.CreateChatMessage(ctx, conv.ID, "user", "Msg 2")
		require.NoError(t, err)

		assert.NotEqual(t, msg1.ID, msg2.ID)
	})
}

// TestListChatMessages tests listing messages via Instance.
func TestListChatMessages(t *testing.T) {
	instance, cleanup := setupTestInstance(t)
	defer cleanup()

	ctx := context.Background()

	conv, err := instance.CreateConversation(ctx, "repo-001", "Messages Test")
	require.NoError(t, err)

	// Create a sequence of messages
	_, err = instance.CreateChatMessage(ctx, conv.ID, "user", "First message")
	require.NoError(t, err)

	_, err = instance.CreateChatMessage(ctx, conv.ID, "assistant", "First response")
	require.NoError(t, err)

	_, err = instance.CreateChatMessage(ctx, conv.ID, "user", "Second message")
	require.NoError(t, err)

	t.Run("list messages in order", func(t *testing.T) {
		messages, err := instance.ListChatMessages(ctx, conv.ID)
		require.NoError(t, err)
		require.Len(t, messages, 3)

		// Verify order and content
		assert.Equal(t, "user", messages[0].Role)
		assert.Equal(t, "First message", messages[0].Content)

		assert.Equal(t, "assistant", messages[1].Role)
		assert.Equal(t, "First response", messages[1].Content)

		assert.Equal(t, "user", messages[2].Role)
		assert.Equal(t, "Second message", messages[2].Content)
	})

	t.Run("list messages for non-existent conversation returns empty", func(t *testing.T) {
		messages, err := instance.ListChatMessages(ctx, "nonexistent-conv")
		require.NoError(t, err)
		assert.Empty(t, messages)
	})
}

// TestConversationMessageFlow tests a realistic flow: create conv, add messages, retrieve.
func TestConversationMessageFlow(t *testing.T) {
	instance, cleanup := setupTestInstance(t)
	defer cleanup()

	ctx := context.Background()

	// Create conversation
	conv, err := instance.CreateConversation(ctx, "repo-001", "Repo Analysis")
	require.NoError(t, err)

	// Add user message
	userMsg, err := instance.CreateChatMessage(ctx, conv.ID, "user", "Analyze this repository")
	require.NoError(t, err)

	// Add assistant response
	assistantMsg, err := instance.CreateChatMessage(ctx, conv.ID, "assistant", "This repo contains 42 files.")
	require.NoError(t, err)

	// Verify conversation can be retrieved with correct state
	retrievedConv, err := instance.GetConversation(ctx, conv.ID)
	require.NoError(t, err)
	assert.Equal(t, conv.ID, retrievedConv.ID)
	assert.Equal(t, "repo-001", retrievedConv.RepositoryID)

	// Verify messages exist and are in order
	messages, err := instance.ListChatMessages(ctx, conv.ID)
	require.NoError(t, err)
	require.Len(t, messages, 2)

	assert.Equal(t, userMsg.ID, messages[0].ID)
	assert.Equal(t, "user", messages[0].Role)

	assert.Equal(t, assistantMsg.ID, messages[1].ID)
	assert.Equal(t, "assistant", messages[1].Role)
}
