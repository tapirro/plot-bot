package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// --- Block contracts (loaded from YAML) ---

type BlockContract struct {
	Type     string `json:"type"`
	Role     string `json:"role"`
	Min      int    `json:"min"`
	Max      int    `json:"max"`
	Format   string `json:"format"`   // list, table, prose, code, any
	Required bool   `json:"required"`
}

// --- YAML parser (lightweight, no deps) ---

func loadContracts() ([]BlockContract, error) {
	// Look for contracts.yaml next to executable, then in root
	paths := []string{
		filepath.Join(filepath.Dir(os.Args[0]), "contracts.yaml"),
		filepath.Join(root, "tools", "ask", "contracts.yaml"),
	}

	var data []byte
	var err error
	for _, p := range paths {
		data, err = os.ReadFile(p)
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("contracts.yaml not found")
	}

	return parseContractsYAML(string(data))
}

func parseContractsYAML(content string) ([]BlockContract, error) {
	var contracts []BlockContract
	scanner := bufio.NewScanner(strings.NewReader(content))

	var current *BlockContract
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip comments and empty
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Top-level key: type.role:
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && strings.HasSuffix(trimmed, ":") {
			if current != nil {
				contracts = append(contracts, *current)
			}
			key := strings.TrimSuffix(trimmed, ":")
			parts := strings.SplitN(key, ".", 2)
			if len(parts) != 2 {
				continue
			}
			current = &BlockContract{
				Type:   parts[0],
				Role:   parts[1],
				Format: "any",
			}
			continue
		}

		// Indented property
		if current != nil && (strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t")) {
			kv := strings.SplitN(trimmed, ":", 2)
			if len(kv) != 2 {
				continue
			}
			k := strings.TrimSpace(kv[0])
			v := strings.TrimSpace(kv[1])
			switch k {
			case "min":
				current.Min, _ = strconv.Atoi(v)
			case "max":
				current.Max, _ = strconv.Atoi(v)
			case "format":
				current.Format = v
			case "required":
				current.Required = v == "true"
			}
		}
	}
	if current != nil {
		contracts = append(contracts, *current)
	}

	return contracts, nil
}

// --- Violation model ---

type Violation struct {
	ArtifactID string `json:"id"`
	Path       string `json:"path"`
	Type       string `json:"type"`
	Role       string `json:"role"`
	Kind       string `json:"kind"`    // stub, bloated, missing, hollow, format
	Actual     int    `json:"actual"`  // actual lines (0 for missing)
	Expected   string `json:"expected"` // "3-30" or "list" etc
	Heading    string `json:"heading,omitempty"`
}

// --- Block relationship helpers ---

// hasSubsections checks if a ## block has ### children before the next ## block.
// A ## with subsections is "structured" — NOT hollow.
func hasSubsections(parent Block, allBlocks []Block) bool {
	// Find LineStart of the next ## block after parent
	nextL2 := -1
	for _, b := range allBlocks {
		if b.Level == 2 && b.LineStart > parent.LineStart {
			nextL2 = b.LineStart
			break
		}
	}
	// Any ### between parent and next ##?
	for _, b := range allBlocks {
		if b.Level == 3 && b.LineStart > parent.LineStart {
			if nextL2 == -1 || b.LineStart < nextL2 {
				return true
			}
		}
	}
	return false
}

// effectiveSize returns the total content lines of a ## block INCLUDING its ### children.
func effectiveSize(parent Block, allBlocks []Block) int {
	total := parent.SizeLines
	nextL2 := -1
	for _, b := range allBlocks {
		if b.Level == 2 && b.LineStart > parent.LineStart {
			nextL2 = b.LineStart
			break
		}
	}
	for _, b := range allBlocks {
		if b.Level == 3 && b.LineStart > parent.LineStart {
			if nextL2 == -1 || b.LineStart < nextL2 {
				total += b.SizeLines + 1 // +1 for heading line
			}
		}
	}
	return total
}

// aggregateHints merges content hints from a ## block and all its ### children.
func aggregateHints(parent Block, allBlocks []Block) ContentHints {
	h := parent.Hints
	nextL2 := -1
	for _, b := range allBlocks {
		if b.Level == 2 && b.LineStart > parent.LineStart {
			nextL2 = b.LineStart
			break
		}
	}
	for _, b := range allBlocks {
		if b.Level == 3 && b.LineStart > parent.LineStart {
			if nextL2 == -1 || b.LineStart < nextL2 {
				h.HasTable = h.HasTable || b.Hints.HasTable
				h.HasCode = h.HasCode || b.Hints.HasCode
				h.HasList = h.HasList || b.Hints.HasList
			}
		}
	}
	return h
}

// --- Audit engine ---

func auditArtifact(a Artifact, blocks []Block, contracts []BlockContract) []Violation {
	var violations []Violation

	// Build role → block map
	roleBlocks := map[string]Block{}
	for _, b := range blocks {
		if b.Role != "" && b.Level == 2 {
			roleBlocks[b.Role] = b
		}
	}

	// Check each contract for this type
	for _, c := range contracts {
		if c.Type != a.Type {
			continue
		}

		b, found := roleBlocks[c.Role]

		// Missing check
		if !found {
			if c.Required {
				violations = append(violations, Violation{
					ArtifactID: a.ID,
					Path:       a.Path,
					Type:       a.Type,
					Role:       c.Role,
					Kind:       "missing",
					Actual:     0,
					Expected:   "required",
				})
			}
			continue
		}

		// Hollow check — but NOT if block has ### subsections (structured, not hollow)
		if b.SizeLines == 0 && !hasSubsections(b, blocks) {
			violations = append(violations, Violation{
				ArtifactID: a.ID,
				Path:       a.Path,
				Type:       a.Type,
				Role:       c.Role,
				Kind:       "hollow",
				Actual:     0,
				Expected:   fmt.Sprintf("%d-%d", c.Min, c.Max),
				Heading:    b.Heading,
			})
			continue
		}

		// Use effective size (includes ### children) for structured blocks
		size := b.SizeLines
		if hasSubsections(b, blocks) {
			size = effectiveSize(b, blocks)
		}

		// Stub check
		if c.Min > 0 && size < c.Min {
			violations = append(violations, Violation{
				ArtifactID: a.ID,
				Path:       a.Path,
				Type:       a.Type,
				Role:       c.Role,
				Kind:       "stub",
				Actual:     size,
				Expected:   fmt.Sprintf(">=%d", c.Min),
				Heading:    b.Heading,
			})
		}

		// Bloated check
		if c.Max > 0 && size > c.Max {
			violations = append(violations, Violation{
				ArtifactID: a.ID,
				Path:       a.Path,
				Type:       a.Type,
				Role:       c.Role,
				Kind:       "bloated",
				Actual:     size,
				Expected:   fmt.Sprintf("<=%d", c.Max),
				Heading:    b.Heading,
			})
		}

		// Format check — aggregate hints from children too
		hints := b.Hints
		if hasSubsections(b, blocks) {
			hints = aggregateHints(b, blocks)
		}
		if c.Format != "any" && c.Format != "" {
			mismatch := false
			switch c.Format {
			case "list":
				mismatch = !hints.HasList
			case "table":
				mismatch = !hints.HasTable
			case "code":
				mismatch = !hints.HasCode
			case "prose":
				// prose = no special format required, just non-empty
			}
			if mismatch && size >= 3 { // Only flag format on non-trivial blocks
				violations = append(violations, Violation{
					ArtifactID: a.ID,
					Path:       a.Path,
					Type:       a.Type,
					Role:       c.Role,
					Kind:       "format",
					Actual:     b.SizeLines,
					Expected:   c.Format,
					Heading:    b.Heading,
				})
			}
		}
	}

	return violations
}

// --- Audit report ---

type AuditReport struct {
	Total       int                    `json:"total"`
	Scanned     int                    `json:"scanned"`
	Clean       int                    `json:"clean"`
	Violations  int                    `json:"violations"`
	ByKind      map[string]int         `json:"by_kind"`
	ByType      map[string]int         `json:"by_type"`
	Items       []Violation            `json:"items"`
	FixPlan     []FixTask              `json:"fix_plan,omitempty"`
}

type FixTask struct {
	Group      string   `json:"group"`
	Action     string   `json:"action"`
	Artifacts  []string `json:"artifacts"`
	Priority   int      `json:"priority"` // 1=high, 2=medium, 3=low
}

func buildAuditReport(violations []Violation, totalArtifacts, scannedArtifacts int) AuditReport {
	byKind := map[string]int{}
	byType := map[string]int{}
	affectedIDs := map[string]bool{}

	for _, v := range violations {
		byKind[v.Kind]++
		byType[v.Type]++
		affectedIDs[v.ArtifactID] = true
	}

	clean := scannedArtifacts - len(affectedIDs)
	if clean < 0 {
		clean = 0
	}

	report := AuditReport{
		Total:      totalArtifacts,
		Scanned:    scannedArtifacts,
		Clean:      clean,
		Violations: len(violations),
		ByKind:     byKind,
		ByType:     byType,
		Items:      violations,
	}

	// Build fix plan — group violations into parallelizable tasks
	report.FixPlan = buildFixPlan(violations)

	return report
}

func buildFixPlan(violations []Violation) []FixTask {
	// Group by (kind, type) → parallelizable subagent tasks
	type groupKey struct{ kind, typ string }
	groups := map[groupKey][]string{}

	for _, v := range violations {
		k := groupKey{v.Kind, v.Type}
		// Deduplicate artifact IDs within group
		found := false
		for _, id := range groups[k] {
			if id == v.ArtifactID {
				found = true
				break
			}
		}
		if !found {
			groups[k] = append(groups[k], v.ArtifactID)
		}
	}

	var tasks []FixTask
	for k, ids := range groups {
		priority := 3
		action := ""
		switch k.kind {
		case "missing":
			priority = 1
			action = fmt.Sprintf("Add missing required blocks to %s artifacts", k.typ)
		case "stub":
			priority = 2
			action = fmt.Sprintf("Expand stub blocks in %s artifacts (below minimum lines)", k.typ)
		case "bloated":
			priority = 2
			action = fmt.Sprintf("Split or compress bloated blocks in %s artifacts (above maximum lines)", k.typ)
		case "hollow":
			priority = 1
			action = fmt.Sprintf("Fill hollow block headers in %s artifacts (0-line headings)", k.typ)
		case "format":
			priority = 3
			action = fmt.Sprintf("Fix format mismatch in %s artifacts (wrong content structure)", k.typ)
		case "provenance":
			priority = 2
			action = fmt.Sprintf("Add confidence+origin+basis to %s artifacts", k.typ)
		}

		sort.Strings(ids)
		tasks = append(tasks, FixTask{
			Group:     fmt.Sprintf("%s/%s", k.kind, k.typ),
			Action:    action,
			Artifacts: ids,
			Priority:  priority,
		})
	}

	// Sort by priority then group name
	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].Priority != tasks[j].Priority {
			return tasks[i].Priority < tasks[j].Priority
		}
		return tasks[i].Group < tasks[j].Group
	})

	return tasks
}

// --- Command: audit ---

func cmdAudit(args []string, mode OutputMode, idx *Index) error {
	typeFilter := ""
	kindFilter := ""
	fixPlanOnly := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--type", "-t":
			i++
			if i < len(args) {
				typeFilter = args[i]
			}
		case "--kind", "-k":
			i++
			if i < len(args) {
				kindFilter = args[i]
			}
		case "--fix":
			fixPlanOnly = true
		}
	}

	contracts, err := loadContracts()
	if err != nil {
		return &FileError{Path: "contracts.yaml", Err: err}
	}

	// Determine which types have contracts
	contractTypes := map[string]bool{}
	for _, c := range contracts {
		contractTypes[c.Type] = true
	}

	var allViolations []Violation
	scanned := 0

	for _, a := range idx.Artifacts {
		if typeFilter != "" && a.Type != typeFilter {
			continue
		}
		if !contractTypes[a.Type] {
			continue
		}
		if strings.Contains(a.Path, "templates/") {
			continue
		}

		content, err := os.ReadFile(filepath.Join(root, a.Path))
		if err != nil {
			continue
		}

		_, bodyStart := parseFrontmatter(string(content))
		blocks := parseBlocks(string(content), bodyStart)
		scanned++

		violations := auditArtifact(a, blocks, contracts)
		if kindFilter != "" {
			var filtered []Violation
			for _, v := range violations {
				if v.Kind == kindFilter {
					filtered = append(filtered, v)
				}
			}
			violations = filtered
		}
		allViolations = append(allViolations, violations...)
	}

	// Provenance violations: types requiring confidence but missing it
	for _, a := range idx.Artifacts {
		if typeFilter != "" && a.Type != typeFilter {
			continue
		}
		if confidenceRequired[a.Type] && a.Confidence == "" {
			if kindFilter != "" && kindFilter != "provenance" {
				continue
			}
			allViolations = append(allViolations, Violation{
				ArtifactID: a.ID,
				Path:       a.Path,
				Type:       a.Type,
				Role:       "frontmatter",
				Kind:       "provenance",
				Actual:     0,
				Expected:   "confidence+origin+basis",
			})
		}
	}

	report := buildAuditReport(allViolations, len(idx.Artifacts), scanned)

	if mode == ModeJSON {
		jsonPrint(report)
		return nil
	}

	// Text output
	if fixPlanOnly {
		outputFixPlan(report, mode)
		return nil
	}

	// Summary line
	pct := compliancePct(report.Clean, report.Scanned)
	fmt.Printf("audit: %d scanned, %d clean (%d%%), %d violations\n",
		report.Scanned, report.Clean, pct, report.Violations)

	// By kind
	if len(report.ByKind) > 0 {
		var kinds []string
		for k := range report.ByKind {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		parts := make([]string, len(kinds))
		for i, k := range kinds {
			parts[i] = fmt.Sprintf("%s:%d", k, report.ByKind[k])
		}
		fmt.Printf("  kinds: %s\n", strings.Join(parts, " "))
	}

	// By type
	if len(report.ByType) > 0 {
		var types []string
		for t := range report.ByType {
			types = append(types, t)
		}
		sort.Strings(types)
		parts := make([]string, len(types))
		for i, t := range types {
			parts[i] = fmt.Sprintf("%s:%d", t, report.ByType[t])
		}
		fmt.Printf("  types: %s\n", strings.Join(parts, " "))
	}

	// Violations detail (wide mode)
	if mode == ModeWide && len(report.Items) > 0 {
		fmt.Println()
		t := (&Table{}).Col("kind", 8).Col("type", 12).Col("role", 14).Col("artifact", 40).Col("detail", 0)
		for _, v := range report.Items {
			detail := ""
			switch v.Kind {
			case "stub":
				detail = fmt.Sprintf("%dL (need %s)", v.Actual, v.Expected)
			case "bloated":
				detail = fmt.Sprintf("%dL (max %s)", v.Actual, v.Expected)
			case "missing":
				detail = "not found"
			case "hollow":
				detail = "0L header"
			case "format":
				detail = fmt.Sprintf("expected %s", v.Expected)
			case "provenance":
				detail = "missing confidence+origin+basis"
			}
			id := v.ArtifactID
			if len(id) > 39 {
				id = id[:39]
			}
			t.Row(v.Kind, v.Type, v.Role, id, detail)
		}
		t.Render(ModeWide)
	} else if len(report.Items) > 0 {
		// Compact: group by artifact
		byArt := map[string][]Violation{}
		var artOrder []string
		for _, v := range report.Items {
			if _, seen := byArt[v.ArtifactID]; !seen {
				artOrder = append(artOrder, v.ArtifactID)
			}
			byArt[v.ArtifactID] = append(byArt[v.ArtifactID], v)
		}
		fmt.Println()
		for _, id := range artOrder {
			vv := byArt[id]
			var parts []string
			for _, v := range vv {
				switch v.Kind {
				case "stub":
					parts = append(parts, fmt.Sprintf("%s:stub(%dL)", v.Role, v.Actual))
				case "bloated":
					parts = append(parts, fmt.Sprintf("%s:bloated(%dL)", v.Role, v.Actual))
				case "missing":
					parts = append(parts, fmt.Sprintf("%s:missing", v.Role))
				case "hollow":
					parts = append(parts, fmt.Sprintf("%s:hollow", v.Role))
				case "format":
					parts = append(parts, fmt.Sprintf("%s:format(%s)", v.Role, v.Expected))
				case "provenance":
					parts = append(parts, "provenance:missing")
				}
			}
			fmt.Printf("  %s  %s\n", id, strings.Join(parts, " "))
		}
	}

	// Fix plan
	if len(report.FixPlan) > 0 {
		fmt.Println()
		outputFixPlan(report, mode)
	}

	// Hint
	if !quietMode && report.Violations > 0 {
		fmt.Printf("\n→ next: ./ask audit --fix  or  ./ask audit -j | jq '.fix_plan'\n")
	}
	return nil
}

func outputFixPlan(report AuditReport, mode OutputMode) {
	if len(report.FixPlan) == 0 {
		fmt.Println("fix plan: no violations")
		return
	}

	fmt.Println("FIX PLAN (parallelizable tasks):")
	for i, task := range report.FixPlan {
		pLabel := "LOW"
		if task.Priority == 1 {
			pLabel = "HIGH"
		} else if task.Priority == 2 {
			pLabel = "MED"
		}
		fmt.Printf("  [%d] %s [%s] %s\n", i+1, pLabel, task.Group, task.Action)
		// Show artifacts (max 10)
		limit := 10
		if len(task.Artifacts) < limit {
			limit = len(task.Artifacts)
		}
		for j := 0; j < limit; j++ {
			fmt.Printf("      - %s\n", task.Artifacts[j])
		}
		if len(task.Artifacts) > limit {
			fmt.Printf("      +%d more\n", len(task.Artifacts)-limit)
		}
	}
}
