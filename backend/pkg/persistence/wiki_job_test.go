package persistence_test

import (
	"context"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/venkatvghub/argus/pkg/models"
	"github.com/venkatvghub/argus/pkg/persistence"
)

func setupTestDB(t *testing.T) *persistence.DB {
	t.Helper()
	dsn := os.Getenv("ARGUS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ARGUS_TEST_DATABASE_URL not set — skipping persistence integration tests")
	}
	db, err := persistence.New(dsn)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func upsertTestRepo(t *testing.T, db *persistence.DB, repoID string) {
	t.Helper()
	err := db.UpsertRepository(context.Background(), models.Repository{
		ID:   repoID,
		Name: repoID,
		Path: "/tmp/" + repoID,
	})
	require.NoError(t, err)
}

func TestCreateWikiJob(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	jobID, err := db.CreateWikiJob(ctx, "test-repo-id", 42)
	require.NoError(t, err)

	// Verify ID is 16 chars hex
	assert.Len(t, jobID, 16)
	for _, ch := range jobID {
		assert.True(t, (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f'),
			"expected lowercase hex, got: %c", ch)
	}

	// Verify GetWikiJob returns the created job with correct state
	job, err := db.GetWikiJob(ctx, jobID)
	require.NoError(t, err)
	assert.Equal(t, jobID, job.ID)
	assert.Equal(t, "test-repo-id", job.RepoID)
	assert.Equal(t, models.WikiJobPending, job.Status)
	assert.Equal(t, 42, job.TotalPages)
	assert.NotZero(t, job.CreatedAt)
	assert.NotZero(t, job.UpdatedAt)
}

func TestUpdateWikiJobStatus(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	jobID, err := db.CreateWikiJob(ctx, "test-repo-id", 10)
	require.NoError(t, err)

	// Verify initial status is pending
	job, err := db.GetWikiJob(ctx, jobID)
	require.NoError(t, err)
	assert.Equal(t, models.WikiJobPending, job.Status)

	// Update to running
	err = db.UpdateWikiJobStatus(ctx, jobID, models.WikiJobRunning)
	require.NoError(t, err)

	// Verify status changed
	job, err = db.GetWikiJob(ctx, jobID)
	require.NoError(t, err)
	assert.Equal(t, models.WikiJobRunning, job.Status)

	// Update to completed
	err = db.UpdateWikiJobStatus(ctx, jobID, models.WikiJobCompleted)
	require.NoError(t, err)

	job, err = db.GetWikiJob(ctx, jobID)
	require.NoError(t, err)
	assert.Equal(t, models.WikiJobCompleted, job.Status)
}

func TestMarkWikiPageComplete_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	jobID, err := db.CreateWikiJob(ctx, "test-repo-id", 5)
	require.NoError(t, err)

	pageID := "page-1"

	// Mark page complete first time
	err = db.MarkWikiPageComplete(ctx, jobID, pageID)
	require.NoError(t, err)

	// Mark same page complete second time (should be idempotent)
	err = db.MarkWikiPageComplete(ctx, jobID, pageID)
	require.NoError(t, err)

	// Verify page appears exactly once
	completed, err := db.GetCompletedWikiPages(ctx, jobID)
	require.NoError(t, err)
	assert.Equal(t, 1, len(completed))
	assert.Contains(t, completed, pageID)
}

func TestGetCompletedWikiPages_Empty(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	jobID, err := db.CreateWikiJob(ctx, "test-repo-id", 10)
	require.NoError(t, err)

	// Get completed pages for a new job (should be empty)
	completed, err := db.GetCompletedWikiPages(ctx, jobID)
	require.NoError(t, err)
	assert.Empty(t, completed)
	assert.Equal(t, 0, len(completed))
}

func TestGetCompletedWikiPages_Multiple(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	jobID, err := db.CreateWikiJob(ctx, "test-repo-id", 5)
	require.NoError(t, err)

	pageIDs := []string{"page-1", "page-2", "page-3"}

	// Mark all pages complete
	for _, pageID := range pageIDs {
		err := db.MarkWikiPageComplete(ctx, jobID, pageID)
		require.NoError(t, err)
	}

	// Verify all pages are in result
	completed, err := db.GetCompletedWikiPages(ctx, jobID)
	require.NoError(t, err)
	assert.Equal(t, 3, len(completed))
	for _, pageID := range pageIDs {
		assert.Contains(t, completed, pageID)
	}
}

func TestListWikiJobs_Order(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	repoID := "test-repo-id"

	// Create first job
	jobID1, err := db.CreateWikiJob(ctx, repoID, 5)
	require.NoError(t, err)

	// Create second job
	jobID2, err := db.CreateWikiJob(ctx, repoID, 10)
	require.NoError(t, err)

	// List jobs for repo
	jobs, err := db.ListWikiJobs(ctx, repoID)
	require.NoError(t, err)

	// Should have exactly 2 jobs
	assert.Equal(t, 2, len(jobs))

	// Most recent should be first (by created_at desc)
	// Both jobs are in the list
	jobIDs := []string{jobs[0].ID, jobs[1].ID}
	assert.Contains(t, jobIDs, jobID1)
	assert.Contains(t, jobIDs, jobID2)

	// Verify order: jobs[0] should have been created after jobs[1]
	// so jobs[0].CreatedAt >= jobs[1].CreatedAt
	assert.GreaterOrEqual(t, jobs[0].CreatedAt, jobs[1].CreatedAt)

	// Verify the jobs have their respective page counts
	// (job with 5 pages and job with 10 pages are both present)
	pageCounts := []int{jobs[0].TotalPages, jobs[1].TotalPages}
	assert.Contains(t, pageCounts, 5)
	assert.Contains(t, pageCounts, 10)
}

func TestListWikiJobs_Empty(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// List jobs for a repo that doesn't have any jobs
	jobs, err := db.ListWikiJobs(ctx, "non-existent-repo")
	require.NoError(t, err)

	// Should return empty slice (nil is acceptable for empty result)
	assert.Empty(t, jobs)
	assert.Equal(t, 0, len(jobs))
}

func TestGetWikiJob_NotFound(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Try to get a job that doesn't exist
	job, err := db.GetWikiJob(ctx, "non-existent-job-id")

	// Should return error
	require.Error(t, err)
	// Job fields should be zero-valued
	assert.Equal(t, "", job.ID)
	assert.Equal(t, "", job.RepoID)
}

func TestWikiJobFullLifecycle(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	repoID := "test-repo-lifecycle"

	// 1. Create a job
	jobID, err := db.CreateWikiJob(ctx, repoID, 100)
	require.NoError(t, err)
	assert.Len(t, jobID, 16)

	// 2. Verify initial state
	job, err := db.GetWikiJob(ctx, jobID)
	require.NoError(t, err)
	assert.Equal(t, models.WikiJobPending, job.Status)
	assert.False(t, job.CreatedAt.IsZero())
	// CreatedAt is set, don't check exact value

	// 3. Update to running
	err = db.UpdateWikiJobStatus(ctx, jobID, models.WikiJobRunning)
	require.NoError(t, err)

	job, err = db.GetWikiJob(ctx, jobID)
	require.NoError(t, err)
	assert.Equal(t, models.WikiJobRunning, job.Status)

	// 4. Mark pages complete as we progress
	for i := 1; i <= 10; i++ {
		pageID := "page-" + strconv.Itoa(i)
		err := db.MarkWikiPageComplete(ctx, jobID, pageID)
		require.NoError(t, err)
	}

	// 5. Verify 10 pages are complete
	completed, err := db.GetCompletedWikiPages(ctx, jobID)
	require.NoError(t, err)
	assert.Equal(t, 10, len(completed))

	// 6. Mark job complete
	err = db.UpdateWikiJobStatus(ctx, jobID, models.WikiJobCompleted)
	require.NoError(t, err)

	job, err = db.GetWikiJob(ctx, jobID)
	require.NoError(t, err)
	assert.Equal(t, models.WikiJobCompleted, job.Status)

	// 7. Verify job is in list
	jobs, err := db.ListWikiJobs(ctx, repoID)
	require.NoError(t, err)
	assert.Equal(t, 1, len(jobs))
	assert.Equal(t, jobID, jobs[0].ID)
	assert.Equal(t, models.WikiJobCompleted, jobs[0].Status)
}

func TestWikiJobMultipleRepos(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	repo1 := "repo-1"
	repo2 := "repo-2"

	// Create jobs for different repos
	job1, err := db.CreateWikiJob(ctx, repo1, 10)
	require.NoError(t, err)

	job2, err := db.CreateWikiJob(ctx, repo2, 20)
	require.NoError(t, err)

	// List jobs for repo1
	jobs1, err := db.ListWikiJobs(ctx, repo1)
	require.NoError(t, err)
	assert.Equal(t, 1, len(jobs1))
	assert.Equal(t, job1, jobs1[0].ID)
	assert.Equal(t, repo1, jobs1[0].RepoID)

	// List jobs for repo2
	jobs2, err := db.ListWikiJobs(ctx, repo2)
	require.NoError(t, err)
	assert.Equal(t, 1, len(jobs2))
	assert.Equal(t, job2, jobs2[0].ID)
	assert.Equal(t, repo2, jobs2[0].RepoID)

	// List jobs for non-existent repo
	jobsNone, err := db.ListWikiJobs(ctx, "non-existent")
	require.NoError(t, err)
	assert.Empty(t, jobsNone)
}
