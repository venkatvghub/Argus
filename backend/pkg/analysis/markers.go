package analysis

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/pkoukk/tiktoken-go"
	"github.com/venkatvghub/argus/pkg/models"
	gonum_graph "gonum.org/v1/gonum/graph"
)

// Regex patterns for Indian Regulatory Compliance (DPDP)
var (
	aadhaarRegex = regexp.MustCompile(`\b\d{12}\b`)
	panRegex     = regexp.MustCompile(`\b[A-Z]{5}[0-9]{4}[A-Z]{1}\b`)
	upiRegex     = regexp.MustCompile(`\b[a-zA-Z0-9.\-_]{2,256}@[a-zA-Z]{2,64}\b`)

	// indianMobileRegex matches Indian 10-digit mobile numbers with optional +91/0091/91 prefix.
	indianMobileRegex = regexp.MustCompile(`(?:(?:\+91|0091|91)[-.\s]?)?[6-9]\d{9}\b`)
	// intlMobileRegex matches international mobile numbers in E.164 format (+<country><number>, 8-15 digits total).
	intlMobileRegex = regexp.MustCompile(`\+[1-9]\d{7,14}\b`)
	// emailRegex matches standard RFC-5321 simplified email addresses, including placeholder domains.
	emailRegex = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
)

// Regex patterns for Dart/Flutter biomarkers (package-level, compiled once)
var (
	dartSetStateAfterAwait  = regexp.MustCompile(`(?s)await\s+.{1,200}setState\s*\(`)
	dartContextAfterAwait   = regexp.MustCompile(`(?s)await\s+.{1,200}(?:Navigator|ScaffoldMessenger|Theme|MediaQuery)\s*\.of\s*\(\s*context\s*\)`)
	dartBrokenCryptoPattern = regexp.MustCompile(`(?i)MD5|SHA1[^2-9]|DES(?:ede)?[^A-Z]`)
)

// Regex patterns for SQL biomarkers (package-level, compiled once)
var (
	sqlConcatPattern = regexp.MustCompile(`(?i)(SELECT|INSERT|UPDATE|DELETE)\s+.{0,100}(\+\s*['"]|['"]\s*\+|CONCAT\s*\()`)
	sqlSelectStar    = regexp.MustCompile(`(?i)SELECT\s+\*\s+FROM`)
	sqlCredsPattern  = regexp.MustCompile(`(?i)(PASSWORD|IDENTIFIED BY)\s+['"][^'"]{3,}['"]`)
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

		// Language-specific regex markers
		if filepath.Ext(file.Path) == ".dart" {
			markers = append(markers, me.checkDartFlutter(file.Path, sContent)...)
		}
		if filepath.Ext(file.Path) == ".sql" {
			markers = append(markers, me.checkSQL(file.Path, sContent)...)
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

	// Indian mobile numbers (DPDP regulated)
	if indianMobileRegex.MatchString(content) {
		markers = append(markers, models.Marker{
			Type:     "dpdp_mobile_exposure",
			Severity: "high",
			Message:  "Potential Indian mobile number exposure detected",
			File:     filePath,
		})
	}

	// International mobile (E.164) — exclude Indian numbers (+91 prefix)
	intlMobiles := intlMobileRegex.FindAllString(content, -1)
	nonIndianIntl := 0
	for _, mobile := range intlMobiles {
		// Exclude +91 which are Indian
		if !strings.HasPrefix(mobile, "+91") {
			nonIndianIntl++
		}
	}
	if nonIndianIntl > 0 {
		markers = append(markers, models.Marker{
			Type:     "pii_mobile_exposure",
			Severity: "medium",
			Message:  "Potential international mobile number (E.164) exposure detected",
			File:     filePath,
		})
	}

	// Email addresses — filter out test/placeholder domains
	emails := emailRegex.FindAllString(content, -1)
	realEmails := filterTestEmails(emails)
	if len(realEmails) > 0 {
		markers = append(markers, models.Marker{
			Type:     "pii_email_exposure",
			Severity: "medium",
			Message:  fmt.Sprintf("Potential email address exposure detected (%d occurrence(s))", len(realEmails)),
			File:     filePath,
		})
	}

	return markers
}

// filterTestEmails removes known test and placeholder email domains from a list.
func filterTestEmails(emails []string) []string {
	testDomains := []string{
		"example.com", "example.org", "example.net",
		"test.com", "test.org", "foo.com", "bar.com",
		"placeholder.com", "noreply.com", "localhost",
	}
	var real []string
	for _, e := range emails {
		isTest := false
		lower := strings.ToLower(e)
		for _, d := range testDomains {
			if strings.HasSuffix(lower, "@"+d) {
				isTest = true
				break
			}
		}
		if !isTest {
			real = append(real, e)
		}
	}
	return real
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
		if node.InternalType() != NodeTypeSymbol {
			continue
		}
		// Only flag exported symbols (uppercase first letter — covers Go and common conventions).
		if len(node.Name) == 0 || !unicode.IsUpper(rune(node.Name[0])) {
			continue
		}
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
	return markers
}

// checkDartFlutter detects Dart/Flutter biomarkers including setState after await, invalid context usage, and weak cryptography.
func (me *MarkerEngine) checkDartFlutter(filePath, content string) []models.Marker {
	var markers []models.Marker
	if dartSetStateAfterAwait.MatchString(content) {
		markers = append(markers, models.Marker{
			Type:     "dart_setstate_after_await",
			Severity: "high",
			Message:  "setState() called after await — may operate on disposed widget",
			File:     filePath,
		})
	}
	if dartContextAfterAwait.MatchString(content) {
		markers = append(markers, models.Marker{
			Type:     "dart_context_after_await",
			Severity: "medium",
			Message:  "BuildContext used after async gap — context may be invalid",
			File:     filePath,
		})
	}
	if dartBrokenCryptoPattern.MatchString(content) {
		markers = append(markers, models.Marker{
			Type:     "dart_broken_crypto",
			Severity: "high",
			Message:  "Weak cryptographic algorithm detected (MD5/SHA1/DES)",
			File:     filePath,
		})
	}
	return markers
}

// checkSQL detects SQL biomarkers including injection risks, overly broad SELECT queries, and hardcoded credentials.
func (me *MarkerEngine) checkSQL(filePath, content string) []models.Marker {
	var markers []models.Marker
	if sqlConcatPattern.MatchString(content) {
		markers = append(markers, models.Marker{
			Type:     "sql_injection_risk",
			Severity: "high",
			Message:  "SQL query built with string concatenation — injection risk",
			File:     filePath,
		})
	}
	if sqlSelectStar.MatchString(content) {
		markers = append(markers, models.Marker{
			Type:     "sql_select_star",
			Severity: "low",
			Message:  "SELECT * reduces AI context efficiency and risks data over-exposure",
			File:     filePath,
		})
	}
	if sqlCredsPattern.MatchString(content) {
		markers = append(markers, models.Marker{
			Type:     "sql_hardcoded_credential",
			Severity: "critical",
			Message:  "Hardcoded credential in SQL file",
			File:     filePath,
		})
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

		// Check both outgoing and incoming file-to-file edges — a file with only
		// incoming structural edges would be incorrectly flagged otherwise.
		hasStructuralEdges := func() bool {
			for _, iter := range []func(id int64) gonum_graph.Nodes{graph.g.From, graph.g.To} {
				neighbors := iter(node.ID())
				for neighbors.Next() {
					if gn, ok := neighbors.Node().(*Node); ok && gn.InternalType() == NodeTypeFile {
						return true
					}
				}
			}
			return false
		}()

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
