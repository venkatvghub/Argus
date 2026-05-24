package analysis

import (
	"github.com/venkatvghub/argus/pkg/models"
)

const (
	// scoreBase is the starting health score for every file.
	scoreBase = 10.0
	// scoreMinFinal is the minimum value Final is clamped to.
	scoreMinFinal = 1.0
	// scoreMaxFinal is the maximum value Final is clamped to (also the base).
	scoreMaxFinal = 10.0
)

// ComputeFileScore groups markers by Category, applies CategoryCaps, clamps Final to [1.0, 10.0].
func ComputeFileScore(file string, markers []models.Marker) models.FileScore {
	deductions := make(map[models.ScoreCategory]float64)

	// Sum raw deductions per category.
	for _, m := range markers {
		if m.Deduction > 0 {
			deductions[m.Category] += m.Deduction
		}
	}

	// Apply per-category caps where defined.
	for cat, total := range deductions {
		if cap, ok := models.CategoryCaps[cat]; ok {
			if total > cap {
				deductions[cat] = cap
			}
		}
	}

	// Sum all capped deductions.
	var totalDeductions float64
	for _, d := range deductions {
		totalDeductions += d
	}

	final := scoreBase - totalDeductions
	if final < scoreMinFinal {
		final = scoreMinFinal
	}
	if final > scoreMaxFinal {
		final = scoreMaxFinal
	}

	return models.FileScore{
		File:        file,
		Base:        scoreBase,
		Final:       final,
		Deductions:  deductions,
		MarkerCount: len(markers),
	}
}

// ComputeRepoScore computes weighted-average of file scores.
// pageRanks maps file path → PageRank value. If nil/empty, uniform weight.
func ComputeRepoScore(scores []models.FileScore, pageRanks map[string]float64) float64 {
	if len(scores) == 0 {
		return scoreMaxFinal
	}

	// Attempt weighted mean if pageRanks is provided.
	if len(pageRanks) > 0 {
		var weightedSum, totalWeight float64
		for _, fs := range scores {
			w := pageRanks[fs.File] // missing key returns 0
			weightedSum += w * fs.Final
			totalWeight += w
		}
		// If every file had weight 0, fall back to uniform.
		if totalWeight > floatCompareEpsilon {
			result := weightedSum / totalWeight
			return clampScore(result)
		}
	}

	// Uniform average fallback.
	var sum float64
	for _, fs := range scores {
		sum += fs.Final
	}
	return clampScore(sum / float64(len(scores)))
}

// clampScore clamps v to [scoreMinFinal, scoreMaxFinal].
func clampScore(v float64) float64 {
	if v < scoreMinFinal {
		return scoreMinFinal
	}
	if v > scoreMaxFinal {
		return scoreMaxFinal
	}
	return v
}
