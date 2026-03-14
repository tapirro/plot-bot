package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// --- Block model ---

type Block struct {
	Slug      string       `json:"slug"`
	Role      string       `json:"role"`
	Heading   string       `json:"heading"`
	Level     int          `json:"level"`
	LineStart int          `json:"line_start"`
	LineEnd   int          `json:"line_end"`
	SizeLines int          `json:"size_lines"`
	SizeBytes int          `json:"size_bytes"`
	Hints     ContentHints `json:"hints"`
}

type ContentHints struct {
	HasTable bool `json:"table"`
	HasCode  bool `json:"code"`
	HasList  bool `json:"list"`
}

// --- Role alias map ---

var roleAliases = map[string]string{
	// Russian slugs → canonical role
	"принятые-решения":                "decisions",
	"решения":                         "decisions",
	"нерешённые-вопросы":              "open-questions",
	"action-items":                    "actions",
	"действия":                        "actions",
	"action-items-с-deep-links":       "actions",
	"referenced-entities":             "references",
	"ссылки":                          "references",
	"оценки-коммуникации":             "participants",
	"оценки-участников":               "participants",
	"3-оценки-участников":             "participants",
	"рейтинги-участников":             "participants",
	"оценка-участников":               "participants",
	"8-осевой-анализ":                 "analysis",
	"2-8-осевой-анализ":               "analysis",
	"классификация-встречи":           "classification",
	"классификация":                   "classification",
	"краткое-саммари":                 "summary",
	"1-краткое-саммари":               "summary",
	"когда-использовать":              "when-to-use",
	"заметки-по-реализации":           "notes",
	"входные-данные":                  "inputs",
	"workflow-выполнения":             "workflow",
	"инсайты":                         "findings",
	"4-принятые-решения":              "decisions",
	"5-нерешённые-вопросы":            "open-questions",
	"6-action-items":                  "actions",
	"суть-предложения":                "offer",
	"вердикт":                         "verdict",
	"ключевые-цитаты":                "quotes",
	"сквозные-темы-всего-периода":     "themes",
	"статус":                          "status",
	// English slugs → canonical role
	"problem":          "problem",
	"solution":         "solution",
	"implementation":   "implementation",
	"adaptation-guide": "adaptation",
	"adaptation":       "adaptation",
	"origin":           "origin",
	"related-patterns": "references",
	"references":       "references",
	"examples":         "examples",
	"traps":            "traps",
	"context":          "context",
	"decision":         "decision",
	"decisions":        "decisions",
	"actions":          "actions",
	"alternatives":     "options",
	"options":          "options",
	"trade-offs":       "tradeoffs",
	"when-to-use":      "when-to-use",
	"summary":          "summary",
	"findings":         "findings",
	"recommendations":  "recommendations",
	"overview":         "summary",
	"notes":            "notes",
	"workflow":         "workflow",
	"status":           "status",
	"analysis":         "analysis",
	"offer":            "offer",
	"verdict":          "verdict",
	"open-questions":   "open-questions",
	"participants":     "participants",
	// Bet / Action / Maintenance / Domain
	"metrics":          "metrics",
	"next-experiment":  "next-experiment",
	"next-step":        "next-experiment",
	"scope":            "scope",
	"norm":             "norm",
	"escalation":       "escalation",
}

// resolveRole maps a slug to a canonical role.
// Priority: explicit "role: title" → alias map → slug itself.
func resolveRole(slug, explicitRole string) string {
	if explicitRole != "" {
		return explicitRole
	}
	if role, ok := roleAliases[slug]; ok {
		return role
	}
	return ""
}

// --- Block schemas per type ---

type BlockSchema struct {
	Required []string // must-have roles
	Optional []string // recommended roles
}

var blockSchemas = map[string]BlockSchema{
	"pattern": {
		Required: []string{"problem", "solution"},
		Optional: []string{"implementation", "adaptation", "origin", "references", "examples", "traps", "when-to-use", "workflow"},
	},
	"methodology": {
		Required: []string{"problem", "solution"},
		Optional: []string{"implementation", "adaptation", "origin", "references", "workflow"},
	},
	"insight": {
		Required: []string{"decisions", "actions", "references"},
		Optional: []string{"analysis", "findings", "participants", "open-questions", "summary", "classification"},
	},
	"deal": {
		Required: []string{"offer"},
		Optional: []string{"verdict", "analysis"},
	},
	"topic": {
		Required: []string{},
		Optional: []string{"context", "options", "evaluation", "decision"},
	},
}

// --- Block parsing ---

// parseBlocks extracts typed blocks from file content.
func parseBlocks(content string, bodyStart int) []Block {
	body := content[bodyStart:]
	lines := strings.Split(body, "\n")
	fmLineCount := strings.Count(content[:bodyStart], "\n")
	var blocks []Block
	var cur *Block

	for i, line := range lines {
		lineNum := fmLineCount + i + 1 // 1-based, absolute

		level := 0
		rest := ""
		if strings.HasPrefix(line, "### ") {
			level = 3
			rest = strings.TrimSpace(line[4:])
		} else if strings.HasPrefix(line, "## ") {
			level = 2
			rest = strings.TrimSpace(line[3:])
		}

		if level >= 2 {
			// Close previous block
			if cur != nil {
				cur.LineEnd = lineNum - 1
				finishBlock(cur, lines, fmLineCount)
				blocks = append(blocks, *cur)
			}

			heading, explicitRole := parseTypedHeading(rest)
			slug := slugify(heading)
			if explicitRole != "" {
				slug = explicitRole
			}
			role := resolveRole(slug, explicitRole)

			cur = &Block{
				Slug:      slug,
				Role:      role,
				Heading:   heading,
				Level:     level,
				LineStart: lineNum + 1,
			}
			continue
		}

		// Content hints for current block
		if cur != nil {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "|") && strings.Contains(trimmed[1:], "|") {
				cur.Hints.HasTable = true
			}
			if strings.HasPrefix(trimmed, "```") {
				cur.Hints.HasCode = true
			}
			if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") ||
				(len(trimmed) > 2 && trimmed[0] >= '0' && trimmed[0] <= '9' && strings.Contains(trimmed[:3], ".")) {
				cur.Hints.HasList = true
			}
		}
	}

	// Close last block
	if cur != nil {
		cur.LineEnd = bodyStart + len(lines)
		finishBlock(cur, lines, fmLineCount)
		blocks = append(blocks, *cur)
	}

	return blocks
}

// finishBlock computes size_lines, size_bytes, trims trailing blanks.
func finishBlock(b *Block, lines []string, fmLineCount int) {
	start := b.LineStart - fmLineCount - 1
	end := b.LineEnd - fmLineCount
	if start < 0 {
		start = 0
	}
	if end > len(lines) {
		end = len(lines)
	}
	// Trim trailing blank lines from count
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	b.LineEnd = fmLineCount + end
	b.SizeLines = end - start
	total := 0
	for i := start; i < end; i++ {
		total += len(lines[i]) + 1
	}
	b.SizeBytes = total
}

// parseTypedHeading parses "role: Human Title" or plain "Human Title".
func parseTypedHeading(s string) (heading, role string) {
	idx := strings.Index(s, ": ")
	if idx > 0 && idx < 30 {
		candidate := s[:idx]
		// Role must be lowercase ascii + hyphens
		valid := true
		for _, c := range candidate {
			if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
				valid = false
				break
			}
		}
		if valid {
			return strings.TrimSpace(s[idx+2:]), candidate
		}
	}
	return s, ""
}

// --- Block extraction ---

// blockMatches checks if a block matches a query by slug, role, or heading.
func blockMatches(b Block, query string) bool {
	return b.Slug == query || b.Role == query || slugify(b.Heading) == query
}

// extractBlockLines returns the content lines for a block from the full-file lines slice.
func extractBlockLines(lines []string, b Block) (contentStart, contentEnd, headingIdx int) {
	headingIdx = b.LineStart - 2 // heading line (0-based)
	contentStart = b.LineStart - 1
	contentEnd = b.LineEnd
	if headingIdx < 0 {
		headingIdx = 0
	}
	if contentStart < 0 {
		contentStart = 0
	}
	if contentEnd > len(lines) {
		contentEnd = len(lines)
	}
	return
}

// getBlockContent extracts content of a single block by role or slug.
func getBlockContent(content string, bodyStart int, query string) (string, bool) {
	blocks := parseBlocks(content, bodyStart)
	for _, b := range blocks {
		if blockMatches(b, query) {
			lines := strings.Split(content, "\n")
			start, end, _ := extractBlockLines(lines, b)
			result := strings.Join(lines[start:end], "\n")
			result = strings.TrimRight(result, "\n ")
			return result, true
		}
	}
	return "", false
}

// getMultiBlockContent extracts multiple blocks by roles/slugs (comma-separated).
func getMultiBlockContent(content string, bodyStart int, queries []string) string {
	blocks := parseBlocks(content, bodyStart)
	lines := strings.Split(content, "\n")
	var parts []string

	for _, q := range queries {
		q = strings.TrimSpace(q)
		for _, b := range blocks {
			if blockMatches(b, q) {
				start, end, headingIdx := extractBlockLines(lines, b)
				// Include heading for multi-block
				if headingIdx >= 0 && headingIdx < len(lines) {
					parts = append(parts, lines[headingIdx])
				}
				part := strings.Join(lines[start:end], "\n")
				parts = append(parts, strings.TrimRight(part, "\n "))
				break
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

// --- Schema validation ---

type SchemaResult struct {
	Type     string   `json:"type"`
	Schema   string   `json:"schema"` // "strict", "recommended", "free"
	Found    []string `json:"found"`
	Missing  []string `json:"missing"`
	Extra    []string `json:"extra"`
	Coverage string   `json:"coverage"` // "3/4"
}

func validateBlockSchema(artType string, blocks []Block) *SchemaResult {
	schema, ok := blockSchemas[artType]
	if !ok {
		return nil
	}

	roles := map[string]bool{}
	for _, b := range blocks {
		if b.Role != "" {
			roles[b.Role] = true
		}
	}

	var found, missing []string
	for _, req := range schema.Required {
		if roles[req] {
			found = append(found, req)
		} else {
			missing = append(missing, req)
		}
	}

	// Extra: roles not in required or optional
	allExpected := map[string]bool{}
	for _, r := range schema.Required {
		allExpected[r] = true
	}
	for _, r := range schema.Optional {
		allExpected[r] = true
	}
	var extra []string
	for r := range roles {
		if !allExpected[r] {
			extra = append(extra, r)
		}
	}

	schemaLevel := "free"
	if len(schema.Required) > 0 {
		if len(missing) == 0 {
			schemaLevel = "strict"
		} else {
			schemaLevel = "recommended"
		}
	}

	return &SchemaResult{
		Type:     artType,
		Schema:   schemaLevel,
		Found:    found,
		Missing:  missing,
		Extra:    extra,
		Coverage: fmt.Sprintf("%d/%d", len(found), len(schema.Required)),
	}
}

// --- Command: blocks ---

func cmdBlocks(args []string, mode OutputMode, idx *Index) error {
	var schemaFlag bool
	var typeFilter string
	var queryParts []string
	roleFilterValue := ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--schema", "-s":
			schemaFlag = true
		case "--role", "-r":
			i++
			if i < len(args) {
				roleFilterValue = args[i]
			}
		case "--type", "-t":
			i++
			if i < len(args) {
				typeFilter = args[i]
			}
		default:
			if !strings.HasPrefix(args[i], "-") {
				queryParts = append(queryParts, args[i])
			}
		}
	}

	query := strings.Join(queryParts, " ")

	// Mode: cross-type block search (--type pattern --role solution)
	if typeFilter != "" && roleFilterValue != "" {
		cmdBlocksCrossSearch(typeFilter, roleFilterValue, mode, idx)
		return nil
	}

	// Mode: single or multi-artifact TOC
	if query == "" {
		return &UsageError{Msg: "usage: ask blocks <id>[,id2,...] [--schema] [--type T --role R]"}
	}

	// Multi-artifact blocks
	ids := strings.Split(query, ",")
	if len(ids) > 1 {
		cmdBlocksMulti(ids, schemaFlag, mode, idx)
		return nil
	}

	// Single artifact
	match := resolveArtifact(query, idx.Artifacts)
	if match == nil {
		return &NotFoundError{What: query}
	}
	if matches, ok := match.([]Artifact); ok {
		fmt.Fprintf(os.Stderr, "%d matches — refine query:\n", len(matches))
		outputList(matches, mode)
		return &UsageError{Msg: fmt.Sprintf("%d matches — refine query", len(matches))}
	}

	a := match.(Artifact)
	content, err := os.ReadFile(filepath.Join(root, a.Path))
	if err != nil {
		return &FileError{Path: a.Path, Err: err}
	}

	_, bodyStart := parseFrontmatter(string(content))
	blocks := parseBlocks(string(content), bodyStart)

	totalLines := 0
	for _, b := range blocks {
		totalLines += b.SizeLines
	}

	if mode == ModeJSON {
		result := map[string]interface{}{
			"id":          a.ID,
			"path":        a.Path,
			"type":        a.Type,
			"total_lines": totalLines,
			"blocks":      blocks,
		}
		if schemaFlag {
			if sr := validateBlockSchema(a.Type, blocks); sr != nil {
				result["schema"] = sr
			}
		}
		data, _ := json.Marshal(result)
		fmt.Println(string(data))
		return nil
	}

	// Text output
	fmt.Printf("%s  (%d blocks, %d lines)\n", a.ID, len(blocks), totalLines)
	for _, b := range blocks {
		prefix := "##"
		if b.Level == 3 {
			prefix = "###"
		}
		roleTag := ""
		if b.Role != "" {
			roleTag = fmt.Sprintf(" [%s]", b.Role)
		}
		hints := formatHints(b.Hints)
		fmt.Printf("  %s %-40s %4dL  %s%s\n", prefix, b.Heading, b.SizeLines, hints, roleTag)
	}

	if schemaFlag {
		fmt.Println()
		if sr := validateBlockSchema(a.Type, blocks); sr != nil {
			outputSchemaResult(sr, mode)
		} else {
			fmt.Println("schema: free (no schema defined for this type)")
		}
	}

	// Hint
	if mode != ModeJSON && len(blocks) > 2 {
		var smallest, largest Block
		for i, b := range blocks {
			if i == 0 || b.SizeLines < smallest.SizeLines {
				smallest = b
			}
			if i == 0 || b.SizeLines > largest.SizeLines {
				largest = b
			}
		}
		if largest.SizeLines > 30 && smallest.SizeLines < largest.SizeLines/2 {
			addr := smallest.Slug
			if smallest.Role != "" {
				addr = smallest.Role
			}
			fmt.Printf("hint: largest block %q is %dL. try: ./ask get %s#%s\n",
				largest.Heading, largest.SizeLines, a.ID, addr)
		}
	}
	return nil
}

func cmdBlocksMulti(ids []string, schemaFlag bool, mode OutputMode, idx *Index) {
	type artBlocks struct {
		ID         string  `json:"id"`
		Type       string  `json:"type"`
		BlockCount int     `json:"block_count"`
		TotalLines int     `json:"total_lines"`
		Blocks     []Block `json:"blocks,omitempty"`
		Schema     *SchemaResult `json:"schema,omitempty"`
	}

	var results []artBlocks

	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		match := resolveArtifact(id, idx.Artifacts)
		if match == nil {
			fmt.Fprintf(os.Stderr, "warning: '%s' not found, skipped\n", id)
			continue
		}
		a, ok := match.(Artifact)
		if !ok {
			if matches, mok := match.([]Artifact); mok && len(matches) > 0 {
				a = matches[0]
			} else {
				continue
			}
		}
		content, err := os.ReadFile(filepath.Join(root, a.Path))
		if err != nil {
			continue
		}
		_, bodyStart := parseFrontmatter(string(content))
		blocks := parseBlocks(string(content), bodyStart)
		totalLines := 0
		for _, b := range blocks {
			totalLines += b.SizeLines
		}
		ab := artBlocks{
			ID:         a.ID,
			Type:       a.Type,
			BlockCount: len(blocks),
			TotalLines: totalLines,
			Blocks:     blocks,
		}
		if schemaFlag {
			ab.Schema = validateBlockSchema(a.Type, blocks)
		}
		results = append(results, ab)
	}

	if mode == ModeJSON {
		data, _ := json.Marshal(map[string]interface{}{"items": results})
		fmt.Println(string(data))
		return
	}

	for _, ab := range results {
		fmt.Printf("%-40s %2d blocks  %4dL\n", ab.ID, ab.BlockCount, ab.TotalLines)
		if mode == ModeWide {
			for _, b := range ab.Blocks {
				roleTag := ""
				if b.Role != "" {
					roleTag = fmt.Sprintf(" [%s]", b.Role)
				}
				fmt.Printf("  %-38s %4dL%s\n", b.Heading, b.SizeLines, roleTag)
			}
		}
		if schemaFlag && ab.Schema != nil {
			outputSchemaResult(ab.Schema, mode)
		}
	}
}

func cmdBlocksCrossSearch(typeFilter, roleFilter string, mode OutputMode, idx *Index) {
	type RoleMatch struct {
		ID        string `json:"id"`
		Role      string `json:"role"`
		Heading   string `json:"heading"`
		SizeLines int    `json:"size_lines"`
	}

	var matches []RoleMatch

	for _, a := range idx.Artifacts {
		if typeFilter != "" && a.Type != typeFilter {
			continue
		}
		content, err := os.ReadFile(filepath.Join(root, a.Path))
		if err != nil {
			continue
		}
		_, bodyStart := parseFrontmatter(string(content))
		blocks := parseBlocks(string(content), bodyStart)
		for _, b := range blocks {
			if b.Role == roleFilter || b.Slug == roleFilter {
				matches = append(matches, RoleMatch{
					ID:        a.ID,
					Role:      b.Role,
					Heading:   b.Heading,
					SizeLines: b.SizeLines,
				})
			}
		}
	}

	if mode == ModeJSON {
		data, _ := json.Marshal(map[string]interface{}{"role": roleFilter, "matches": matches, "total": len(matches)})
		fmt.Println(string(data))
		return
	}

	for _, m := range matches {
		if mode == ModeWide {
			fmt.Printf("%-40s %-30s %4dL  [%s]\n", m.ID, m.Heading, m.SizeLines, m.Role)
		} else {
			fmt.Printf("%s\t%s\t%dL\n", m.ID, m.Role, m.SizeLines)
		}
	}
	fmt.Printf("%d blocks with role '%s'\n", len(matches), roleFilter)
}

// --- Output helpers ---

func formatHints(h ContentHints) string {
	var parts []string
	if h.HasTable {
		parts = append(parts, "table")
	}
	if h.HasCode {
		parts = append(parts, "code")
	}
	if h.HasList {
		parts = append(parts, "list")
	}
	if len(parts) == 0 {
		return "[text]"
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func outputSchemaResult(sr *SchemaResult, mode OutputMode) {
	if mode == ModeJSON {
		return // already included in parent JSON
	}
	if len(sr.Missing) == 0 && len(sr.Found) > 0 {
		parts := make([]string, len(sr.Found))
		for i, f := range sr.Found {
			parts[i] = "+" + f
		}
		fmt.Printf("schema: %s %s (%s)\n", sr.Coverage, sr.Schema, strings.Join(parts, " "))
	} else if len(sr.Missing) > 0 {
		mparts := make([]string, len(sr.Missing))
		for i, m := range sr.Missing {
			mparts[i] = "-" + m
		}
		fmt.Printf("schema: %s (missing: %s)\n", sr.Coverage, strings.Join(mparts, " "))
	}
}
