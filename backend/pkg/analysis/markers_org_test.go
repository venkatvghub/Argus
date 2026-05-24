package analysis

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/venkatvghub/argus/pkg/models"
)

// TestCheckGitOrgMarkers_DeveloperCongestion verifies developer_congestion marker is emitted.
func TestCheckGitOrgMarkers_DeveloperCongestion(t *testing.T) {
	files := []models.FileNode{
		{
			Path:        "handler.go",
			IsFile:      true,
			Churn:       3,
			AuthorCount: 6, // >= 5 threshold
		},
	}

	me := &MarkerEngine{}
	markers := me.checkGitOrgMarkers(files)

	require.NotEmpty(t, markers)
	found := false
	for _, m := range markers {
		if m.Type == "developer_congestion" && m.File == "handler.go" {
			found = true
			assert.Equal(t, "medium", m.Severity)
			assert.Equal(t, models.ScoreCatOrg, m.Category)
			assert.Equal(t, devCongestionDeduction, m.Deduction)
			break
		}
	}
	assert.True(t, found, "developer_congestion marker not found")
}

// TestCheckGitOrgMarkers_BelowThreshold verifies no congestion marker below threshold.
func TestCheckGitOrgMarkers_BelowThreshold(t *testing.T) {
	files := []models.FileNode{
		{
			Path:        "handler.go",
			IsFile:      true,
			Churn:       3,
			AuthorCount: 4, // < 5 threshold
		},
	}

	me := &MarkerEngine{}
	markers := me.checkGitOrgMarkers(files)

	for _, m := range markers {
		assert.NotEqual(t, m.Type, "developer_congestion")
	}
}

// TestCheckGitOrgMarkers_KnowledgeLoss verifies knowledge_loss marker for old inactive author.
func TestCheckGitOrgMarkers_KnowledgeLoss(t *testing.T) {
	primaryLastCommit := time.Now().Add(-200 * 24 * time.Hour) // 200 days ago

	files := []models.FileNode{
		{
			Path:                    "core.go",
			IsFile:                  true,
			Churn:                   8, // >= 5 threshold
			PrimaryAuthorLastCommit: primaryLastCommit,
		},
	}

	me := &MarkerEngine{}
	markers := me.checkGitOrgMarkers(files)

	require.NotEmpty(t, markers)
	found := false
	for _, m := range markers {
		if m.Type == "knowledge_loss" && m.File == "core.go" {
			found = true
			assert.Equal(t, "high", m.Severity)
			assert.Equal(t, models.ScoreCatOrg, m.Category)
			assert.Equal(t, knowledgeLossDeduction, m.Deduction)
			break
		}
	}
	assert.True(t, found, "knowledge_loss marker not found")
}

// TestCheckGitOrgMarkers_KnowledgeLoss_RecentCommit verifies no loss for recent commit.
func TestCheckGitOrgMarkers_KnowledgeLoss_RecentCommit(t *testing.T) {
	primaryLastCommit := time.Now().Add(-10 * 24 * time.Hour) // 10 days ago (< 180 threshold)

	files := []models.FileNode{
		{
			Path:                    "core.go",
			IsFile:                  true,
			Churn:                   8,
			PrimaryAuthorLastCommit: primaryLastCommit,
		},
	}

	me := &MarkerEngine{}
	markers := me.checkGitOrgMarkers(files)

	for _, m := range markers {
		assert.NotEqual(t, m.Type, "knowledge_loss", "recent commit should not trigger knowledge_loss")
	}
}

// TestCheckGitOrgMarkers_KnowledgeLoss_LowChurn verifies no loss for low churn.
func TestCheckGitOrgMarkers_KnowledgeLoss_LowChurn(t *testing.T) {
	primaryLastCommit := time.Now().Add(-200 * 24 * time.Hour) // 200 days ago

	files := []models.FileNode{
		{
			Path:                    "core.go",
			IsFile:                  true,
			Churn:                   2, // < 5 threshold
			PrimaryAuthorLastCommit: primaryLastCommit,
		},
	}

	me := &MarkerEngine{}
	markers := me.checkGitOrgMarkers(files)

	for _, m := range markers {
		assert.NotEqual(t, m.Type, "knowledge_loss", "low churn should not trigger knowledge_loss")
	}
}

// TestCheckGitOrgMarkers_ZeroPrimaryAuthorLastCommit verifies zero-value timestamp skips check.
func TestCheckGitOrgMarkers_ZeroPrimaryAuthorLastCommit(t *testing.T) {
	files := []models.FileNode{
		{
			Path:                    "core.go",
			IsFile:                  true,
			Churn:                   8,
			PrimaryAuthorLastCommit: time.Time{}, // zero value
		},
	}

	me := &MarkerEngine{}
	markers := me.checkGitOrgMarkers(files)

	for _, m := range markers {
		assert.NotEqual(t, m.Type, "knowledge_loss", "zero timestamp should skip check")
	}
}

// TestCheckGitOrgMarkers_SkipDirectoryNodes verifies directories are skipped.
func TestCheckGitOrgMarkers_SkipDirectoryNodes(t *testing.T) {
	files := []models.FileNode{
		{
			Path:        "cmd/",
			IsFile:      false, // directory
			AuthorCount: 6,
			Churn:       8,
			PrimaryAuthorLastCommit: time.Now().Add(-200 * 24 * time.Hour),
		},
	}

	me := &MarkerEngine{}
	markers := me.checkGitOrgMarkers(files)

	for _, m := range markers {
		assert.NotEqual(t, m.File, "cmd/", "directory nodes should be skipped")
	}
}

// TestCheckGitOrgMarkers_CongestionAndLossOnSameFile verifies both markers can coexist.
func TestCheckGitOrgMarkers_CongestionAndLossOnSameFile(t *testing.T) {
	files := []models.FileNode{
		{
			Path:                    "critical.go",
			IsFile:                  true,
			AuthorCount:             6,
			Churn:                   8,
			PrimaryAuthorLastCommit: time.Now().Add(-200 * 24 * time.Hour),
		},
	}

	me := &MarkerEngine{}
	markers := me.checkGitOrgMarkers(files)

	var hasCongest, hasLoss bool
	for _, m := range markers {
		if m.Type == "developer_congestion" {
			hasCongest = true
		}
		if m.Type == "knowledge_loss" {
			hasLoss = true
		}
	}

	assert.True(t, hasCongest, "developer_congestion should be emitted")
	assert.True(t, hasLoss, "knowledge_loss should be emitted")
}
