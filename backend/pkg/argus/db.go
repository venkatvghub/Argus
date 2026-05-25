package argus

import (
	"context"

	"github.com/venkatvghub/argus/pkg/models"
	"github.com/venkatvghub/argus/pkg/persistence"
)

// dbStore is the persistence interface used by Instance.
// persistence.DB satisfies it implicitly; noopDB satisfies it for tests.
type dbStore interface {
	Close() error
	UpsertRepository(ctx context.Context, repo models.Repository) error
	ClearRepoLastCommit(ctx context.Context, repoID string) error
	GetRepository(ctx context.Context, repoID string) (models.Repository, error)
	ListRepositories(ctx context.Context) ([]models.Repository, error)
	DeleteRepository(ctx context.Context, repoID string) error
	UpsertMarkers(ctx context.Context, repoID string, markers []models.Marker) error
	LoadAllMarkers(ctx context.Context) (map[string][]models.Marker, error)
	CreateWikiJob(ctx context.Context, repoID string, totalPages int) (string, error)
	UpdateWikiJobStatus(ctx context.Context, jobID string, status models.WikiJobStatus) error
	MarkWikiPageComplete(ctx context.Context, jobID, pageID string) error
	GetCompletedWikiPages(ctx context.Context, jobID string) (map[string]struct{}, error)
	GetWikiJob(ctx context.Context, jobID string) (models.WikiJob, error)
	ListWikiJobs(ctx context.Context, repoID string) ([]models.WikiJob, error)
	UpsertWikiPage(ctx context.Context, page models.WikiPage) error
	ListWikiPages(ctx context.Context, repoID string) ([]models.WikiPage, error)
	GetWikiPage(ctx context.Context, pageID string) (models.WikiPage, error)
	CreateJob(ctx context.Context, job models.Job) error
	GetJob(ctx context.Context, jobID string) (models.Job, error)
	ListJobs(ctx context.Context, repoID string) ([]models.Job, error)
	UpdateJobStatus(ctx context.Context, jobID, status, progress, errMsg string) error
	CreateConversation(ctx context.Context, conv models.Conversation) error
	GetConversation(ctx context.Context, convID string) (models.Conversation, error)
	ListConversations(ctx context.Context, repoID string) ([]models.Conversation, error)
	DeleteConversation(ctx context.Context, convID string) error
	IncrementMessageCount(ctx context.Context, convID string) error
	CreateChatMessage(ctx context.Context, msg models.ChatMessage) error
	ListChatMessages(ctx context.Context, convID string) ([]models.ChatMessage, error)
	RecordLLMCost(ctx context.Context, rec persistence.LLMCostRecord) error
	ListLLMCosts(ctx context.Context, repoID, by string) ([]persistence.CostGroup, error)
	GetLLMCostSummary(ctx context.Context, repoID string) (persistence.CostSummary, error)
	SaveProviderPricing(ctx context.Context, provider string, pricing map[string][2]float64) error
	LoadProviderPricing(ctx context.Context, provider string) (map[string][2]float64, error)
	UpsertRepoFiles(ctx context.Context, repoID string, files []models.FileNode) error
	GetRepoFiles(ctx context.Context, repoID string) ([]models.FileNode, error)
	UpsertRepoSymbols(ctx context.Context, repoID string, symbols []models.Symbol) error
	GetRepoSymbols(ctx context.Context, repoID string) ([]models.Symbol, error)
}
