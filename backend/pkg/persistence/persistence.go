// Package persistence provides the PostgreSQL-backed data access layer for Argus.
package persistence

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/venkatvghub/argus/pkg/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// DB wraps the GORM database connection.
type DB struct {
	db *gorm.DB
}

// -- GORM models ----------------------------------------------------------

type repositoryRow struct {
	ID         string    `gorm:"column:id;type:text;primaryKey"`
	Name       string    `gorm:"column:name;type:text;not null"`
	LocalPath  string    `gorm:"column:local_path;type:text;not null"`
	LastCommit string    `gorm:"column:last_commit;type:text"`
	CreatedAt  time.Time `gorm:"column:created_at;type:timestamptz;autoCreateTime"`
	UpdatedAt  time.Time `gorm:"column:updated_at;type:timestamptz;autoUpdateTime"`
}

func (*repositoryRow) TableName() string { return "repositories" }

type markerRow struct {
	ID         int64   `gorm:"column:id;type:bigserial;primaryKey"`
	RepoID     string  `gorm:"column:repo_id;type:text;not null;index"`
	File       string  `gorm:"column:file;type:text;not null"`
	Type       string  `gorm:"column:type;type:text;not null"`
	Severity   string  `gorm:"column:severity;type:text;not null"`
	Message    string  `gorm:"column:message;type:text;not null"`
	Line       int     `gorm:"column:line;type:integer"`
	Deduction  float64 `gorm:"column:deduction;type:double precision"`
	Category   string  `gorm:"column:category;type:text"`
	Suggestion string  `gorm:"column:suggestion;type:text"`
}

func (*markerRow) TableName() string { return "markers" }

type wikiJobRow struct {
	ID         string    `gorm:"column:id;type:text;primaryKey"`
	RepoID     string    `gorm:"column:repo_id;type:text;not null;index"`
	Status     string    `gorm:"column:status;type:text;not null"`
	TotalPages int       `gorm:"column:total_pages;type:integer;not null"`
	CreatedAt  time.Time `gorm:"column:created_at;type:timestamptz;autoCreateTime"`
	UpdatedAt  time.Time `gorm:"column:updated_at;type:timestamptz;autoUpdateTime"`
}

func (*wikiJobRow) TableName() string { return "wiki_jobs" }

type wikiJobPageRow struct {
	JobID  string `gorm:"column:job_id;type:text;primaryKey"`
	PageID string `gorm:"column:page_id;type:text;primaryKey"`
}

func (*wikiJobPageRow) TableName() string { return "wiki_job_pages" }

type wikiPageRow struct {
	ID        string    `gorm:"column:id;type:text;primaryKey"`
	RepoID    string    `gorm:"column:repo_id;type:text;not null;index"`
	JobID     string    `gorm:"column:job_id;type:text;not null"`
	Type      string    `gorm:"column:type;type:text;not null"`
	Subject   string    `gorm:"column:subject;type:text;not null"`
	Content   string    `gorm:"column:content;type:text"`
	Model     string    `gorm:"column:model;type:text;not null;default:''"`
	Level     int       `gorm:"column:level;type:integer"`
	CreatedAt time.Time `gorm:"column:created_at;type:timestamptz;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;type:timestamptz;autoUpdateTime"`
}

func (*wikiPageRow) TableName() string { return "wiki_pages" }

type jobRow struct {
	ID        string    `gorm:"column:id;type:text;primaryKey"`
	RepoID    string    `gorm:"column:repo_id;type:text;not null;index"`
	Type      string    `gorm:"column:type;type:text;not null"`
	Status    string    `gorm:"column:status;type:text;not null"`
	Progress  string    `gorm:"column:progress;type:text"`
	Error     string    `gorm:"column:error;type:text"`
	CreatedAt time.Time `gorm:"column:created_at;type:timestamptz;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;type:timestamptz;autoUpdateTime"`
}

func (*jobRow) TableName() string { return "jobs" }

type conversationRow struct {
	ID           string    `gorm:"column:id;type:text;primaryKey"`
	RepoID       string    `gorm:"column:repo_id;type:text;not null;index"`
	Title        string    `gorm:"column:title;type:text"`
	MessageCount int       `gorm:"column:message_count;type:integer;not null;default:0"`
	CreatedAt    time.Time `gorm:"column:created_at;type:timestamptz;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;type:timestamptz;autoUpdateTime"`
}

func (*conversationRow) TableName() string { return "conversations" }

type chatMessageRow struct {
	ID             string    `gorm:"column:id;type:text;primaryKey"`
	ConversationID string    `gorm:"column:conversation_id;type:text;not null;index"`
	Role           string    `gorm:"column:role;type:text;not null"`
	Content        string    `gorm:"column:content;type:text;not null"`
	CreatedAt      time.Time `gorm:"column:created_at;type:timestamptz;autoCreateTime"`
}

func (*chatMessageRow) TableName() string { return "chat_messages" }

type llmCostRow struct {
	ID           int64     `gorm:"column:id;type:bigserial;primaryKey"`
	RepoID       string    `gorm:"column:repo_id;type:text;not null;index"`
	Model        string    `gorm:"column:model;type:text;not null"`
	Operation    string    `gorm:"column:operation;type:text;not null"`
	InputTokens  int       `gorm:"column:input_tokens;type:integer;not null;default:0"`
	OutputTokens int       `gorm:"column:output_tokens;type:integer;not null;default:0"`
	CostUSD      float64   `gorm:"column:cost_usd;type:double precision;not null;default:0"`
	CalledAt     time.Time `gorm:"column:called_at;type:timestamptz;autoCreateTime"`
}

func (*llmCostRow) TableName() string { return "llm_costs" }

// providerPricingRow caches live model pricing fetched from provider discovery APIs.
// One row per provider (e.g. "openrouter"); pricing stored as JSON blob.
type providerPricingRow struct {
	Provider    string    `gorm:"column:provider;type:text;primaryKey"`
	PricingJSON []byte    `gorm:"column:pricing_json;type:jsonb;not null"`
	FetchedAt   time.Time `gorm:"column:fetched_at;type:timestamptz;autoUpdateTime"`
}

func (*providerPricingRow) TableName() string { return "provider_pricing" }

type repoFileRow struct {
	ID                      int64      `gorm:"column:id;type:bigserial;primaryKey"`
	RepoID                  string     `gorm:"column:repo_id;type:text;not null;uniqueIndex:idx_repo_files_repo_path"`
	Path                    string     `gorm:"column:path;type:text;not null;uniqueIndex:idx_repo_files_repo_path"`
	Language                string     `gorm:"column:language;type:text"`
	Churn                   int        `gorm:"column:churn;type:integer;not null;default:0"`
	Ownership               float64    `gorm:"column:ownership;type:double precision;not null;default:0"`
	AuthorCount             int        `gorm:"column:author_count;type:integer;not null;default:0"`
	LineCoverage            float64    `gorm:"column:line_coverage;type:double precision;not null;default:0"`
	Size                    int64      `gorm:"column:size;type:bigint;not null;default:0"`
	PrimaryAuthorLastCommit *time.Time `gorm:"column:primary_author_last_commit;type:timestamptz"`
}

func (*repoFileRow) TableName() string { return "repo_files" }

type repoSymbolRow struct {
	ID       int64  `gorm:"column:id;type:bigserial;primaryKey"`
	RepoID   string `gorm:"column:repo_id;type:text;not null;index"`
	Name     string `gorm:"column:name;type:text;not null"`
	Type     string `gorm:"column:type;type:text;not null"`
	FilePath string `gorm:"column:file_path;type:text;not null"`
	Line     int    `gorm:"column:line;type:integer;not null;default:0"`
	EndLine  int    `gorm:"column:end_line;type:integer;not null;default:0"`
}

func (*repoSymbolRow) TableName() string { return "repo_symbols" }

// -- Lifecycle ------------------------------------------------------------

// New opens a PostgreSQL connection at dsn and runs AutoMigrate for all tables.
func New(dsn string) (*DB, error) {
	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Silent),
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres: %w", err)
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	if err := gdb.AutoMigrate(
		&repositoryRow{},
		&markerRow{},
		&wikiJobRow{},
		&wikiJobPageRow{},
		&wikiPageRow{},
		&jobRow{},
		&conversationRow{},
		&chatMessageRow{},
		&llmCostRow{},
		&providerPricingRow{},
		&repoFileRow{},
		&repoSymbolRow{},
	); err != nil {
		return nil, fmt.Errorf("AutoMigrate failed: %w", err)
	}

	return &DB{db: gdb}, nil
}

// Close gracefully shuts down the database connection.
func (d *DB) Close() error {
	sqlDB, err := d.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// -- Repositories ---------------------------------------------------------

func (d *DB) UpsertRepository(ctx context.Context, repo models.Repository) error {
	row := repositoryRow{
		ID:         repo.ID,
		Name:       repo.Name,
		LocalPath:  repo.Path,
		LastCommit: repo.LastCommit,
	}
	return d.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"name", "local_path", "last_commit", "updated_at"}),
		}).
		Create(&row).Error
}

func (d *DB) ClearRepoLastCommit(ctx context.Context, repoID string) error {
	return d.db.WithContext(ctx).
		Model(&repositoryRow{}).
		Where("id = ?", repoID).
		Update("last_commit", "").Error
}

func (d *DB) GetRepository(ctx context.Context, repoID string) (models.Repository, error) {
	var row repositoryRow
	err := d.db.WithContext(ctx).Where("id = ?", repoID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Repository{}, fmt.Errorf("repository %q: %w", repoID, gorm.ErrRecordNotFound)
		}
		return models.Repository{}, err
	}
	return models.Repository{
		ID:         row.ID,
		Name:       row.Name,
		Path:       row.LocalPath,
		LastCommit: row.LastCommit,
		CreatedAt:  row.CreatedAt,
	}, nil
}

func (d *DB) ListRepositories(ctx context.Context) ([]models.Repository, error) {
	var rows []repositoryRow
	if err := d.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, err
	}
	repos := make([]models.Repository, 0, len(rows))
	for _, r := range rows {
		repos = append(repos, models.Repository{
			ID:         r.ID,
			Name:       r.Name,
			Path:       r.LocalPath,
			LastCommit: r.LastCommit,
			CreatedAt:  r.CreatedAt,
		})
	}
	return repos, nil
}

func (d *DB) DeleteRepository(ctx context.Context, repoID string) error {
	return d.db.WithContext(ctx).Where("id = ?", repoID).Delete(&repositoryRow{}).Error
}

// -- Markers --------------------------------------------------------------

func (d *DB) UpsertMarkers(ctx context.Context, repoID string, markers []models.Marker) error {
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("repo_id = ?", repoID).Delete(&markerRow{}).Error; err != nil {
			return err
		}
		if len(markers) == 0 {
			return nil
		}
		rows := make([]markerRow, 0, len(markers))
		for _, m := range markers {
			rows = append(rows, markerRow{
				RepoID:     repoID,
				File:       m.File,
				Type:       m.Type,
				Severity:   m.Severity,
				Message:    m.Message,
				Line:       m.Line,
				Deduction:  m.Deduction,
				Category:   string(m.Category),
				Suggestion: m.Suggestion,
			})
		}
		return tx.Create(&rows).Error
	})
}

func (d *DB) LoadAllMarkers(ctx context.Context) (map[string][]models.Marker, error) {
	var rows []markerRow
	if err := d.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[string][]models.Marker)
	for _, r := range rows {
		result[r.RepoID] = append(result[r.RepoID], models.Marker{
			File:       r.File,
			Type:       r.Type,
			Severity:   r.Severity,
			Message:    r.Message,
			Line:       r.Line,
			Deduction:  r.Deduction,
			Category:   models.ScoreCategory(r.Category),
			Suggestion: r.Suggestion,
		})
	}
	return result, nil
}

// -- Wiki jobs ------------------------------------------------------------

func (d *DB) CreateWikiJob(ctx context.Context, repoID string, totalPages int) (string, error) {
	id := fmt.Sprintf("%x", sha256.Sum256([]byte(repoID+time.Now().UTC().String())))[:16]
	row := wikiJobRow{
		ID:         id,
		RepoID:     repoID,
		Status:     string(models.WikiJobPending),
		TotalPages: totalPages,
	}
	if err := d.db.WithContext(ctx).Create(&row).Error; err != nil {
		return "", err
	}
	return id, nil
}

func (d *DB) UpdateWikiJobStatus(ctx context.Context, jobID string, status models.WikiJobStatus) error {
	return d.db.WithContext(ctx).
		Model(&wikiJobRow{}).
		Where("id = ?", jobID).
		Updates(map[string]any{"status": string(status), "updated_at": time.Now().UTC()}).Error
}

func (d *DB) MarkWikiPageComplete(ctx context.Context, jobID, pageID string) error {
	row := wikiJobPageRow{JobID: jobID, PageID: pageID}
	return d.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&row).Error
}

func (d *DB) GetCompletedWikiPages(ctx context.Context, jobID string) (map[string]struct{}, error) {
	var rows []wikiJobPageRow
	if err := d.db.WithContext(ctx).Where("job_id = ?", jobID).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		result[r.PageID] = struct{}{}
	}
	return result, nil
}

func (d *DB) GetWikiJob(ctx context.Context, jobID string) (models.WikiJob, error) {
	var row wikiJobRow
	err := d.db.WithContext(ctx).Where("id = ?", jobID).First(&row).Error
	if err != nil {
		return models.WikiJob{}, err
	}
	return models.WikiJob{
		ID:         row.ID,
		RepoID:     row.RepoID,
		Status:     models.WikiJobStatus(row.Status),
		TotalPages: row.TotalPages,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}, nil
}

func (d *DB) ListWikiJobs(ctx context.Context, repoID string) ([]models.WikiJob, error) {
	var rows []wikiJobRow
	if err := d.db.WithContext(ctx).
		Where("repo_id = ?", repoID).
		Order("created_at DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	jobs := make([]models.WikiJob, 0, len(rows))
	for _, r := range rows {
		jobs = append(jobs, models.WikiJob{
			ID:         r.ID,
			RepoID:     r.RepoID,
			Status:     models.WikiJobStatus(r.Status),
			TotalPages: r.TotalPages,
			CreatedAt:  r.CreatedAt,
			UpdatedAt:  r.UpdatedAt,
		})
	}
	return jobs, nil
}

// -- Wiki pages -----------------------------------------------------------

func (d *DB) UpsertWikiPage(ctx context.Context, page models.WikiPage) error {
	row := wikiPageRow{
		ID:      page.ID,
		RepoID:  page.RepoID,
		JobID:   page.JobID,
		Type:    page.Type,
		Subject: page.Subject,
		Content: page.Content,
		Model:   page.Model,
		Level:   page.Level,
	}
	return d.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"content", "model", "updated_at"}),
		}).
		Create(&row).Error
}

func (d *DB) ListWikiPages(ctx context.Context, repoID string) ([]models.WikiPage, error) {
	var rows []wikiPageRow
	if err := d.db.WithContext(ctx).
		Where("repo_id = ?", repoID).
		Order("level, type, subject").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	pages := make([]models.WikiPage, 0, len(rows))
	for _, r := range rows {
		pages = append(pages, models.WikiPage{
			ID:        r.ID,
			RepoID:    r.RepoID,
			JobID:     r.JobID,
			Type:      r.Type,
			Subject:   r.Subject,
			Content:   r.Content,
			Model:     r.Model,
			Level:     r.Level,
			CreatedAt: r.CreatedAt,
			UpdatedAt: r.UpdatedAt,
		})
	}
	return pages, nil
}

func (d *DB) GetWikiPage(ctx context.Context, pageID string) (models.WikiPage, error) {
	var row wikiPageRow
	if err := d.db.WithContext(ctx).Where("id = ?", pageID).First(&row).Error; err != nil {
		return models.WikiPage{}, err
	}
	return models.WikiPage{
		ID:        row.ID,
		RepoID:    row.RepoID,
		JobID:     row.JobID,
		Type:      row.Type,
		Subject:   row.Subject,
		Content:   row.Content,
		Model:     row.Model,
		Level:     row.Level,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}, nil
}

// -- Jobs -----------------------------------------------------------------

func (d *DB) CreateJob(ctx context.Context, job models.Job) error {
	row := jobRow{
		ID:       job.ID,
		RepoID:   job.RepoID,
		Type:     job.Type,
		Status:   string(job.Status),
		Progress: job.Progress,
		Error:    job.Error,
	}
	return d.db.WithContext(ctx).Create(&row).Error
}

func (d *DB) GetJob(ctx context.Context, jobID string) (models.Job, error) {
	var row jobRow
	if err := d.db.WithContext(ctx).Where("id = ?", jobID).First(&row).Error; err != nil {
		return models.Job{}, err
	}
	return models.Job{
		ID:        row.ID,
		RepoID:    row.RepoID,
		Type:      row.Type,
		Status:    models.JobStatus(row.Status),
		Progress:  row.Progress,
		Error:     row.Error,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}, nil
}

func (d *DB) ListJobs(ctx context.Context, repoID string) ([]models.Job, error) {
	q := d.db.WithContext(ctx).Order("created_at DESC")
	if repoID != "" {
		q = q.Where("repo_id = ?", repoID)
	}
	var rows []jobRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	jobs := make([]models.Job, 0, len(rows))
	for _, r := range rows {
		jobs = append(jobs, models.Job{
			ID:        r.ID,
			RepoID:    r.RepoID,
			Type:      r.Type,
			Status:    models.JobStatus(r.Status),
			Progress:  r.Progress,
			Error:     r.Error,
			CreatedAt: r.CreatedAt,
			UpdatedAt: r.UpdatedAt,
		})
	}
	return jobs, nil
}

func (d *DB) UpdateJobStatus(ctx context.Context, jobID, status, progress, errMsg string) error {
	return d.db.WithContext(ctx).
		Model(&jobRow{}).
		Where("id = ?", jobID).
		Updates(map[string]any{
			"status":     status,
			"progress":   progress,
			"error":      errMsg,
			"updated_at": time.Now().UTC(),
		}).Error
}

// -- Conversations --------------------------------------------------------

func (d *DB) CreateConversation(ctx context.Context, conv models.Conversation) error {
	row := conversationRow{
		ID:     conv.ID,
		RepoID: conv.RepositoryID,
		Title:  conv.Title,
	}
	return d.db.WithContext(ctx).Create(&row).Error
}

func (d *DB) GetConversation(ctx context.Context, convID string) (models.Conversation, error) {
	var row conversationRow
	if err := d.db.WithContext(ctx).Where("id = ?", convID).First(&row).Error; err != nil {
		return models.Conversation{}, err
	}
	return models.Conversation{
		ID:           row.ID,
		RepositoryID: row.RepoID,
		Title:        row.Title,
		MessageCount: row.MessageCount,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}, nil
}

func (d *DB) ListConversations(ctx context.Context, repoID string) ([]models.Conversation, error) {
	var rows []conversationRow
	if err := d.db.WithContext(ctx).
		Where("repo_id = ?", repoID).
		Order("updated_at DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	convs := make([]models.Conversation, 0, len(rows))
	for _, r := range rows {
		convs = append(convs, models.Conversation{
			ID:           r.ID,
			RepositoryID: r.RepoID,
			Title:        r.Title,
			MessageCount: r.MessageCount,
			CreatedAt:    r.CreatedAt,
			UpdatedAt:    r.UpdatedAt,
		})
	}
	return convs, nil
}

func (d *DB) DeleteConversation(ctx context.Context, convID string) error {
	return d.db.WithContext(ctx).Where("id = ?", convID).Delete(&conversationRow{}).Error
}

func (d *DB) IncrementMessageCount(ctx context.Context, convID string) error {
	return d.db.WithContext(ctx).
		Model(&conversationRow{}).
		Where("id = ?", convID).
		Updates(map[string]any{
			"message_count": gorm.Expr("message_count + 1"),
			"updated_at":    time.Now().UTC(),
		}).Error
}

// -- Chat messages --------------------------------------------------------

func (d *DB) CreateChatMessage(ctx context.Context, msg models.ChatMessage) error {
	row := chatMessageRow{
		ID:             msg.ID,
		ConversationID: msg.ConversationID,
		Role:           msg.Role,
		Content:        msg.Content,
	}
	return d.db.WithContext(ctx).Create(&row).Error
}

func (d *DB) ListChatMessages(ctx context.Context, convID string) ([]models.ChatMessage, error) {
	var rows []chatMessageRow
	if err := d.db.WithContext(ctx).
		Where("conversation_id = ?", convID).
		Order("created_at ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	msgs := make([]models.ChatMessage, 0, len(rows))
	for _, r := range rows {
		msgs = append(msgs, models.ChatMessage{
			ID:             r.ID,
			ConversationID: r.ConversationID,
			Role:           r.Role,
			Content:        r.Content,
			CreatedAt:      r.CreatedAt,
		})
	}
	return msgs, nil
}

// -- LLM costs ------------------------------------------------------------

// LLMCostRecord is a single LLM call cost entry to persist.
type LLMCostRecord struct {
	RepoID       string
	Model        string
	Operation    string
	InputTokens  int
	OutputTokens int
	CostUSD      float64
}

// CostGroup is an aggregated cost row returned by ListLLMCosts.
type CostGroup struct {
	Group        string  `json:"group"`
	Calls        int     `json:"calls"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

// CostSummary is the aggregate cost totals for a repo.
type CostSummary struct {
	TotalCostUSD      float64 `json:"total_cost_usd"`
	TotalCalls        int     `json:"total_calls"`
	TotalInputTokens  int     `json:"total_input_tokens"`
	TotalOutputTokens int     `json:"total_output_tokens"`
	Since             *string `json:"since"`
}

func (d *DB) RecordLLMCost(ctx context.Context, rec LLMCostRecord) error {
	op := rec.Operation
	if op == "" {
		op = "chat"
	}
	row := llmCostRow{
		RepoID:       rec.RepoID,
		Model:        rec.Model,
		Operation:    op,
		InputTokens:  rec.InputTokens,
		OutputTokens: rec.OutputTokens,
		CostUSD:      rec.CostUSD,
	}
	return d.db.WithContext(ctx).Create(&row).Error
}

func (d *DB) ListLLMCosts(ctx context.Context, repoID, by string) ([]CostGroup, error) {
	var groupExpr string
	switch by {
	case "model":
		groupExpr = "model"
	case "operation":
		groupExpr = "operation"
	default:
		groupExpr = "DATE(called_at)"
	}
	q := fmt.Sprintf(`
		SELECT %s AS grp, COUNT(*) AS calls, SUM(input_tokens), SUM(output_tokens), SUM(cost_usd)
		FROM llm_costs WHERE repo_id = $1
		GROUP BY grp ORDER BY grp DESC`, groupExpr)
	rows, err := d.db.WithContext(ctx).Raw(q, repoID).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []CostGroup
	for rows.Next() {
		var g CostGroup
		if err := rows.Scan(&g.Group, &g.Calls, &g.InputTokens, &g.OutputTokens, &g.CostUSD); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func (d *DB) GetLLMCostSummary(ctx context.Context, repoID string) (CostSummary, error) {
	var summary CostSummary
	var since *string
	row := d.db.WithContext(ctx).Raw(`
		SELECT COUNT(*), COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0), COALESCE(SUM(cost_usd),0.0), MIN(called_at)::text
		FROM llm_costs WHERE repo_id = $1`, repoID).Row()
	if err := row.Scan(&summary.TotalCalls, &summary.TotalInputTokens, &summary.TotalOutputTokens, &summary.TotalCostUSD, &since); err != nil {
		return summary, err
	}
	summary.Since = since
	return summary, nil
}

// -- Provider pricing cache -----------------------------------------------

// SaveProviderPricing persists a live pricing map for a provider (upsert by provider name).
func (d *DB) SaveProviderPricing(ctx context.Context, provider string, pricing map[string][2]float64) error {
	if len(pricing) == 0 {
		return nil
	}
	data, err := json.Marshal(pricing)
	if err != nil {
		return fmt.Errorf("marshal pricing: %w", err)
	}
	return d.db.WithContext(ctx).Save(&providerPricingRow{
		Provider:    provider,
		PricingJSON: data,
	}).Error
}

// LoadProviderPricing retrieves the cached pricing map for a provider.
// Returns nil map (not an error) when no cached pricing exists.
func (d *DB) LoadProviderPricing(ctx context.Context, provider string) (map[string][2]float64, error) {
	var row providerPricingRow
	if err := d.db.WithContext(ctx).First(&row, "provider = ?", provider).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	var pricing map[string][2]float64
	if err := json.Unmarshal(row.PricingJSON, &pricing); err != nil {
		return nil, fmt.Errorf("unmarshal pricing: %w", err)
	}
	return pricing, nil
}

// -- Repo files -----------------------------------------------------------

func (d *DB) UpsertRepoFiles(ctx context.Context, repoID string, files []models.FileNode) error {
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("repo_id = ?", repoID).Delete(&repoFileRow{}).Error; err != nil {
			return err
		}
		if len(files) == 0 {
			return nil
		}
		rows := make([]repoFileRow, 0, len(files))
		for _, f := range files {
			var pac *time.Time
			if !f.PrimaryAuthorLastCommit.IsZero() {
				t := f.PrimaryAuthorLastCommit
				pac = &t
			}
			rows = append(rows, repoFileRow{
				RepoID:                  repoID,
				Path:                    f.Path,
				Language:                f.Language,
				Churn:                   f.Churn,
				Ownership:               f.Ownership,
				AuthorCount:             f.AuthorCount,
				LineCoverage:            f.LineCoverage,
				Size:                    f.Size,
				PrimaryAuthorLastCommit: pac,
			})
		}
		return tx.CreateInBatches(&rows, 500).Error
	})
}

func (d *DB) GetRepoFiles(ctx context.Context, repoID string) ([]models.FileNode, error) {
	var rows []repoFileRow
	if err := d.db.WithContext(ctx).
		Where("repo_id = ?", repoID).
		Order("churn DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	files := make([]models.FileNode, 0, len(rows))
	for _, r := range rows {
		f := models.FileNode{
			Path:         r.Path,
			Language:     r.Language,
			Churn:        r.Churn,
			Ownership:    r.Ownership,
			AuthorCount:  r.AuthorCount,
			LineCoverage: r.LineCoverage,
			Size:         r.Size,
			IsFile:       true,
		}
		if r.PrimaryAuthorLastCommit != nil {
			f.PrimaryAuthorLastCommit = *r.PrimaryAuthorLastCommit
		}
		files = append(files, f)
	}
	return files, nil
}

// -- Repo symbols ---------------------------------------------------------

func (d *DB) UpsertRepoSymbols(ctx context.Context, repoID string, symbols []models.Symbol) error {
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("repo_id = ?", repoID).Delete(&repoSymbolRow{}).Error; err != nil {
			return err
		}
		if len(symbols) == 0 {
			return nil
		}
		rows := make([]repoSymbolRow, 0, len(symbols))
		for _, s := range symbols {
			rows = append(rows, repoSymbolRow{
				RepoID:   repoID,
				Name:     s.Name,
				Type:     string(s.Type),
				FilePath: s.FilePath,
				Line:     s.Line,
				EndLine:  s.EndLine,
			})
		}
		return tx.CreateInBatches(&rows, 500).Error
	})
}

func (d *DB) GetRepoSymbols(ctx context.Context, repoID string) ([]models.Symbol, error) {
	var rows []repoSymbolRow
	if err := d.db.WithContext(ctx).Where("repo_id = ?", repoID).Find(&rows).Error; err != nil {
		return nil, err
	}
	symbols := make([]models.Symbol, 0, len(rows))
	for _, r := range rows {
		symbols = append(symbols, models.Symbol{
			Name:     r.Name,
			Type:     models.SymbolType(r.Type),
			FilePath: r.FilePath,
			Line:     r.Line,
			EndLine:  r.EndLine,
		})
	}
	return symbols, nil
}
