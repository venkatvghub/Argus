package analysis

import (
	"fmt"
	"time"

	"github.com/venkatvghub/argus/pkg/models"
)

// checkGitOrgMarkers detects developer_congestion and knowledge_loss risks.
func (me *MarkerEngine) checkGitOrgMarkers(files []models.FileNode) []models.Marker {
	var markers []models.Marker

	for _, file := range files {
		if !file.IsFile {
			continue
		}

		// developer_congestion: ≥ 5 distinct authors in last 90 days
		if file.AuthorCount >= devCongestionAuthorThreshold {
			markers = append(markers, models.Marker{
				Type:      "developer_congestion",
				Severity:  "medium",
				Message:   fmt.Sprintf("File touched by %d distinct authors in the last 90 days", file.AuthorCount),
				File:      file.Path,
				Deduction: devCongestionDeduction,
				Category:  models.ScoreCatOrg,
			})
		}

		// knowledge_loss: primary author inactive ≥ 180 days AND churn ≥ 5
		if !file.PrimaryAuthorLastCommit.IsZero() &&
			time.Since(file.PrimaryAuthorLastCommit) >= knowledgeLossDays*24*time.Hour &&
			file.Churn >= knowledgeLossChurnThreshold {
			markers = append(markers, models.Marker{
				Type:      "knowledge_loss",
				Severity:  "high",
				Message:   fmt.Sprintf("Primary author's last commit was %.0f days ago on a high-churn file", time.Since(file.PrimaryAuthorLastCommit).Hours()/24),
				File:      file.Path,
				Deduction: knowledgeLossDeduction,
				Category:  models.ScoreCatOrg,
			})
		}
	}

	return markers
}
