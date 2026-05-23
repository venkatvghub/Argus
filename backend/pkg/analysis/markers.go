package analysis

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkoukk/tiktoken-go"
	"github.com/venkatvghub/argus/pkg/models"
)

// Regex patterns for Indian Regulatory Compliance (DPDP)
var (
	aadhaarRegex = regexp.MustCompile(`\b\d{12}\b`)
	panRegex     = regexp.MustCompile(`\b[A-Z]{5}[0-9]{4}[A-Z]{1}\b`)
	upiRegex     = regexp.MustCompile(`\b[a-zA-Z0-9.\-_]{2,256}@[a-zA-Z]{2,64}\b`)
)

// MarkerEngine performs regulatory and efficiency analysis.
type MarkerEngine struct {
	repoPath string
}

// NewMarkerEngine creates a new MarkerEngine.
func NewMarkerEngine(repoPath string) *MarkerEngine {
	return &MarkerEngine{repoPath: repoPath}
}

// Run executes all markers on the provided files, symbols, and the assembled graph.
func (me *MarkerEngine) Run(files []models.FileNode, symbols []models.Symbol, graph *GraphEngine) []models.Marker {
	var markers []models.Marker

	tkm, _ := tiktoken.GetEncoding("cl100k_base")

	for _, file := range files {
		if !file.IsFile {
			continue
		}
		content, err := os.ReadFile(filepath.Join(me.repoPath, file.Path))
		if err != nil {
			continue
		}
		sContent := string(content)

		// 4.2 Indian Regulatory Compliance (DPDP)
		markers = append(markers, me.checkPII(file.Path, sContent)...)
		markers = append(markers, me.checkUntrackedConsent(file.Path, sContent)...)
		markers = append(markers, me.checkRBILogging(file.Path, sContent)...)
		markers = append(markers, me.checkDataSovereignty(file.Path, sContent)...)

		// 4.3 AI-Agent Efficiency (Token Bloat part)
		if tkm != nil {
			markers = append(markers, me.checkTokenBloat(file.Path, sContent, tkm)...)
		}
	}

	// 4.3 AI-Agent Efficiency (Structural Graph-based markers)
	// Always run — only needs symbols, not graph
	markers = append(markers, me.detectHallucinationBait(symbols)...)

	if graph != nil {
		markers = append(markers, me.detectZombieExports(graph)...)
		markers = append(markers, me.detectPhantomCoupling(files, graph)...)
	}

	return markers
}

func (me *MarkerEngine) checkPII(filePath, content string) []models.Marker {
	var markers []models.Marker
	if aadhaarRegex.MatchString(content) {
		markers = append(markers, models.Marker{
			Type:     "dpdp_pii_exposure",
			Severity: "high",
			Message:  "Potential Aadhaar number exposure detected",
			File:     filePath,
		})
	}
	if panRegex.MatchString(content) {
		markers = append(markers, models.Marker{
			Type:     "dpdp_pii_exposure",
			Severity: "high",
			Message:  "Potential PAN number exposure detected",
			File:     filePath,
		})
	}
	if upiRegex.MatchString(content) {
		markers = append(markers, models.Marker{
			Type:     "dpdp_pii_exposure",
			Severity: "medium",
			Message:  "Potential UPI ID exposure detected",
			File:     filePath,
		})
	}
	return markers
}

func (me *MarkerEngine) checkUntrackedConsent(filePath, content string) []models.Marker {
	var markers []models.Marker
	mutationKeywords := []string{"setState", "updateUser", "db.Save", "db.Update", "mutation"}
	hasMutation := false
	for _, kw := range mutationKeywords {
		if strings.Contains(content, kw) {
			hasMutation = true
			break
		}
	}

	if hasMutation && !strings.Contains(strings.ToLower(content), "consent") {
		markers = append(markers, models.Marker{
			Type:     "untracked_consent_mutation",
			Severity: "medium",
			Message:  "State mutation detected without explicit consent check heuristic",
			File:     filePath,
		})
	}
	return markers
}

func (me *MarkerEngine) checkRBILogging(filePath, content string) []models.Marker {
	var markers []models.Marker
	if strings.Contains(content, "Payment") || strings.Contains(content, "Transaction") {
		if !strings.Contains(content, "CorrelationID") && !strings.Contains(content, "RequestID") {
			markers = append(markers, models.Marker{
				Type:     "rbi_logger_audit_gap",
				Severity: "high",
				Message:  "Financial handler missing correlation ID in logs",
				File:     filePath,
			})
		}
	}
	return markers
}

func (me *MarkerEngine) checkDataSovereignty(filePath, content string) []models.Marker {
	var markers []models.Marker
	// Heuristic: check for non-Indian region identifiers in AWS/GCP configs
	regions := []regexp.Regexp{
		*regexp.MustCompile(`us-east-1|us-west-2|eu-central-1`),
	}
	for _, reg := range regions {
		if reg.MatchString(content) && (aadhaarRegex.MatchString(content) || panRegex.MatchString(content)) {
			markers = append(markers, models.Marker{
				Type:     "data_sovereignty_leak",
				Severity: "critical",
				Message:  "PII data potentially routed to non-Indian regions",
				File:     filePath,
			})
		}
	}
	return markers
}

func (me *MarkerEngine) checkTokenBloat(filePath, content string, tkm *tiktoken.Tiktoken) []models.Marker {
	var markers []models.Marker
	tokens := tkm.Encode(content, nil, nil)
	lineCount := strings.Count(content, "\n") + 1
	if lineCount > 0 {
		density := float64(len(tokens)) / float64(lineCount)
		if density > 50.0 {
			markers = append(markers, models.Marker{
				Type:     "token_bloat",
				Severity: "low",
				Message:  fmt.Sprintf("High token density detected: %.2f tokens/line", density),
				File:     filePath,
			})
		}
	}
	return markers
}

func (me *MarkerEngine) detectHallucinationBait(symbols []models.Symbol) []models.Marker {
	var markers []models.Marker
	fileSymbols := make(map[string]map[string]bool)

	for _, s := range symbols {
		if _, ok := fileSymbols[s.FilePath]; !ok {
			fileSymbols[s.FilePath] = make(map[string]bool)
		}
		if fileSymbols[s.FilePath][s.Name] {
			markers = append(markers, models.Marker{
				Type:     "hallucination_bait",
				Severity: "medium",
				Message:  fmt.Sprintf("Overlapping symbol name '%s' in the same module/file", s.Name),
				File:     s.FilePath,
				Line:     s.Line,
			})
		}
		fileSymbols[s.FilePath][s.Name] = true
	}
	return markers
}

func (me *MarkerEngine) detectZombieExports(graph *GraphEngine) []models.Marker {
	var markers []models.Marker
	nodes := graph.GetNodes()
	for _, node := range nodes {
		if node.InternalType() == NodeTypeSymbol {
			// A "zombie" export is a symbol node with zero incoming "calls" edges
			// but it's exported (Heuristic: symbols we capture are usually the ones we care about).
			if graph.g.To(node.ID()).Len() == 0 {
				markers = append(markers, models.Marker{
					Type:     "zombie_exports",
					Severity: "low",
					Message:  fmt.Sprintf("Exported symbol '%s' has zero incoming call edges", node.Name),
					File:     node.Symbol().FilePath,
					Line:     node.Symbol().Line,
				})
			}
		}
	}
	return markers
}

func (me *MarkerEngine) detectPhantomCoupling(files []models.FileNode, graph *GraphEngine) []models.Marker {
	var markers []models.Marker
	// Phantom Coupling: high co-change activity (high churn + low single-author ownership)
	// combined with zero structural file-to-file edges in the graph.
	const churnThreshold = 5
	const ownershipThreshold = 0.5

	for _, f := range files {
		if !f.IsFile {
			continue
		}
		if f.Churn < churnThreshold {
			continue
		}
		if f.Ownership >= ownershipThreshold {
			continue
		}

		// Check if this file has any structural edges TO other file nodes.
		node, ok := graph.GetNode(fileKey(f.Path))
		if !ok {
			continue
		}

		hasStructuralEdges := false
		neighbors := graph.g.From(node.ID())
		for neighbors.Next() {
			n := neighbors.Node()
			if gn, ok := n.(*Node); ok && gn.InternalType() == NodeTypeFile {
				hasStructuralEdges = true
				break
			}
		}

		if !hasStructuralEdges {
			ownershipPercent := int(f.Ownership * 100)
			markers = append(markers, models.Marker{
				Type:     "phantom_coupling",
				Severity: "medium",
				Message:  fmt.Sprintf("Phantom coupling detected in %s: Churn=%d, Ownership=%d%%", f.Path, f.Churn, ownershipPercent),
				File:     f.Path,
			})
		}
	}
	return markers
}
