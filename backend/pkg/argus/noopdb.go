package argus

import (
	"context"
	"errors"

	"github.com/venkatvghub/argus/pkg/config"
	"github.com/venkatvghub/argus/pkg/models"
	"github.com/venkatvghub/argus/pkg/persistence"
)

var errNoopDB = errors.New("noopDB: no persistence configured")

// noopDB is a do-nothing implementation of dbStore used in tests that do not
// exercise persistence logic. All writes succeed silently; all reads return the
// zero value and errNoopDB.
type noopDB struct{}

func (noopDB) Close() error                                              { return nil }
func (noopDB) UpsertRepository(_ context.Context, _ models.Repository) error { return nil }
func (noopDB) ClearRepoLastCommit(_ context.Context, _ string) error    { return nil }
func (noopDB) GetRepository(_ context.Context, _ string) (models.Repository, error) {
	return models.Repository{}, errNoopDB
}
func (noopDB) ListRepositories(_ context.Context) ([]models.Repository, error) {
	return nil, nil
}
func (noopDB) DeleteRepository(_ context.Context, _ string) error { return nil }
func (noopDB) UpsertMarkers(_ context.Context, _ string, _ []models.Marker) error { return nil }
func (noopDB) LoadAllMarkers(_ context.Context) (map[string][]models.Marker, error) {
	return map[string][]models.Marker{}, nil
}
func (noopDB) CreateWikiJob(_ context.Context, _ string, _ int) (string, error) {
	return "", errNoopDB
}
func (noopDB) UpdateWikiJobStatus(_ context.Context, _ string, _ models.WikiJobStatus) error {
	return nil
}
func (noopDB) MarkWikiPageComplete(_ context.Context, _, _ string) error { return nil }
func (noopDB) GetCompletedWikiPages(_ context.Context, _ string) (map[string]struct{}, error) {
	return map[string]struct{}{}, nil
}
func (noopDB) GetWikiJob(_ context.Context, _ string) (models.WikiJob, error) {
	return models.WikiJob{}, errNoopDB
}
func (noopDB) ListWikiJobs(_ context.Context, _ string) ([]models.WikiJob, error) {
	return nil, nil
}
func (noopDB) UpsertWikiPage(_ context.Context, _ models.WikiPage) error { return nil }
func (noopDB) ListWikiPages(_ context.Context, _ string) ([]models.WikiPage, error) {
	return nil, nil
}
func (noopDB) GetWikiPage(_ context.Context, _ string) (models.WikiPage, error) {
	return models.WikiPage{}, errNoopDB
}
func (noopDB) CreateJob(_ context.Context, _ models.Job) error              { return nil }
func (noopDB) GetJob(_ context.Context, _ string) (models.Job, error)       { return models.Job{}, errNoopDB }
func (noopDB) ListJobs(_ context.Context, _ string) ([]models.Job, error)   { return nil, nil }
func (noopDB) UpdateJobStatus(_ context.Context, _, _, _, _ string) error   { return nil }
func (noopDB) CreateConversation(_ context.Context, _ models.Conversation) error { return nil }
func (noopDB) GetConversation(_ context.Context, _ string) (models.Conversation, error) {
	return models.Conversation{}, errNoopDB
}
func (noopDB) ListConversations(_ context.Context, _ string) ([]models.Conversation, error) {
	return nil, nil
}
func (noopDB) DeleteConversation(_ context.Context, _ string) error      { return nil }
func (noopDB) IncrementMessageCount(_ context.Context, _ string) error   { return nil }
func (noopDB) CreateChatMessage(_ context.Context, _ models.ChatMessage) error { return nil }
func (noopDB) ListChatMessages(_ context.Context, _ string) ([]models.ChatMessage, error) {
	return nil, nil
}
func (noopDB) RecordLLMCost(_ context.Context, _ persistence.LLMCostRecord) error { return nil }
func (noopDB) ListLLMCosts(_ context.Context, _, _ string) ([]persistence.CostGroup, error) {
	return nil, nil
}
func (noopDB) GetLLMCostSummary(_ context.Context, _ string) (persistence.CostSummary, error) {
	return persistence.CostSummary{}, nil
}
func (noopDB) SaveProviderPricing(_ context.Context, _ string, _ map[string][2]float64) error {
	return nil
}
func (noopDB) LoadProviderPricing(_ context.Context, _ string) (map[string][2]float64, error) {
	return nil, errNoopDB
}
func (noopDB) UpsertRepoFiles(_ context.Context, _ string, _ []models.FileNode) error { return nil }
func (noopDB) GetRepoFiles(_ context.Context, _ string) ([]models.FileNode, error) {
	return nil, nil
}
func (noopDB) UpsertRepoSymbols(_ context.Context, _ string, _ []models.Symbol) error { return nil }
func (noopDB) GetRepoSymbols(_ context.Context, _ string) ([]models.Symbol, error) {
	return nil, nil
}

// NewForTest creates an Instance backed by a no-op in-memory store.
// Intended for unit tests that don't exercise persistence logic.
func NewForTest(ctx context.Context, cfg *config.Config) (*Instance, error) {
	return newWithDB(ctx, cfg, noopDB{})
}
