package argus

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"

	"github.com/venkatvghub/argus/pkg/models"
)

// newShortID returns a 16-char hex string from 8 crypto-random bytes.
func newShortID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Extremely unlikely; fall back to a deterministic but unique counter isn't
		// available here, so we panic only in test/debug contexts.
		panic("crypto/rand unavailable: " + err.Error())
	}
	return fmt.Sprintf("%x", b)
}

// CreateConversation creates a new conversation for a repository.
func (i *Instance) CreateConversation(ctx context.Context, repoID, title string) (models.Conversation, error) {
	conv := models.Conversation{
		ID:           newShortID(),
		RepositoryID: repoID,
		Title:        title,
	}
	if err := i.db.CreateConversation(ctx, conv); err != nil {
		return models.Conversation{}, err
	}
	return conv, nil
}

// GetConversation returns a conversation by ID.
func (i *Instance) GetConversation(ctx context.Context, convID string) (models.Conversation, error) {
	c, err := i.db.GetConversation(ctx, convID)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrConversationNotFound
	}
	return c, err
}

// ListConversations returns all conversations for a repository.
func (i *Instance) ListConversations(ctx context.Context, repoID string) ([]models.Conversation, error) {
	convs, err := i.db.ListConversations(ctx, repoID)
	if err != nil {
		return nil, err
	}
	if convs == nil {
		return []models.Conversation{}, nil
	}
	return convs, nil
}

// DeleteConversation deletes a conversation and all its messages.
func (i *Instance) DeleteConversation(ctx context.Context, convID string) error {
	return i.db.DeleteConversation(ctx, convID)
}

// CreateChatMessage persists a new chat message and increments the conversation message count.
func (i *Instance) CreateChatMessage(ctx context.Context, convID, role, content string) (models.ChatMessage, error) {
	msg := models.ChatMessage{
		ID:             newShortID(),
		ConversationID: convID,
		Role:           role,
		Content:        content,
	}
	if err := i.db.CreateChatMessage(ctx, msg); err != nil {
		return models.ChatMessage{}, err
	}
	if err := i.db.IncrementMessageCount(ctx, convID); err != nil {
		return models.ChatMessage{}, fmt.Errorf("increment message count for conversation %q: %w", convID, err)
	}
	return msg, nil
}

// ListChatMessages returns all messages for a conversation.
func (i *Instance) ListChatMessages(ctx context.Context, convID string) ([]models.ChatMessage, error) {
	msgs, err := i.db.ListChatMessages(ctx, convID)
	if err != nil {
		return nil, err
	}
	if msgs == nil {
		return []models.ChatMessage{}, nil
	}
	return msgs, nil
}
