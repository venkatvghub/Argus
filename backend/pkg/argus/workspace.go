package argus

import (
	"context"
	"time"

	"github.com/venkatvghub/argus/pkg/models"
)

// WorkspaceResponse matches the frontend WorkspaceResponse type.
type WorkspaceResponse struct {
	ID           string               `json:"id"`
	Name         string               `json:"name"`
	Repositories []models.Repository  `json:"repositories"`
	CreatedAt    time.Time            `json:"created_at"`
}

// GetWorkspace returns a single workspace listing all indexed repositories.
func (i *Instance) GetWorkspace(ctx context.Context) (WorkspaceResponse, error) {
	repos, err := i.db.ListRepositories(ctx)
	if err != nil {
		return WorkspaceResponse{}, err
	}
	if repos == nil {
		repos = []models.Repository{}
	}

	name := "default"
	if i.cfg != nil && i.cfg.AppName != "" {
		name = i.cfg.AppName
	}

	return WorkspaceResponse{
		ID:           "default",
		Name:         name,
		Repositories: repos,
		CreatedAt:    time.Now().UTC(),
	}, nil
}
