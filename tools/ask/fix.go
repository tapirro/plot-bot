package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// --- Fix plan model ---

type FixContext struct {
	ArtifactID string            `json:"id"`
	Path       string            `json:"path"`
	Type       string            `json:"type"`
	Violations []Violation       `json:"violations"`
	Context    map[string]string `json:"context"` // role → current content excerpt
}

type FixPlanOutput struct {
	Tasks    []FixContext `json:"tasks"`
	Total    int          `json:"total"`
	EditHint string       `json:"edit_hint"`
}

// --- Command: fix ---

func cmdFix(args []string, mode OutputMode, idx *Index) {
	planOnly := false
	applyStubs := false
	dispatch := false
	dryRun := false
	typeFilter := ""
	kindFilter := ""
	batchSize := 8

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--plan":
			planOnly = true
		case "--stub":
			applyStubs = true
		case "--dispatch":
			dispatch = true
		case "--dry-run", "-n":
			dryRun = true
		case "--batch-size":
			i++
			if i < len(args) {
				fmt.Sscanf(args[i], "%d", &batchSize)
			}
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
		}
	}

	contracts, err := loadContracts()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	contractTypes := map[string]bool{}
	for _, c := range contracts {
		contractTypes[c.Type] = true
	}

	// Collect violations with context
	var tasks []FixContext

	for _, a := range idx.Artifacts {
		if typeFilter != "" && a.Type != typeFilter {
			continue
		}
		if !contractTypes[a.Type] {
			continue
		}
		// Skip template files — they have placeholder stubs by design
		if strings.Contains(a.Path, "templates/") {
			continue
		}

		content, err := os.ReadFile(filepath.Join(root, a.Path))
		if err != nil {
			continue
		}

		_, bodyStart := parseFrontmatter(string(content))
		blocks := parseBlocks(string(content), bodyStart)

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

		if len(violations) == 0 {
			continue
		}

		// Extract context for each violation
		ctx := map[string]string{}
		for _, v := range violations {
			// Current block content (if exists)
			if v.Kind != "missing" {
				bc, found := getBlockContent(string(content), bodyStart, v.Role)
				if found {
					// Truncate to first 20 lines
					lines := strings.Split(bc, "\n")
					if len(lines) > 20 {
						lines = lines[:20]
						lines = append(lines, "...")
					}
					ctx[v.Role+"_current"] = strings.Join(lines, "\n")
				}
			}
		}

		// Extract implementation excerpt (most useful for expansion)
		// If ## Implementation is hollow (content in ### children), collect from subsections
		implContent, found := getBlockContent(string(content), bodyStart, "implementation")
		if found && strings.TrimSpace(implContent) != "" {
			lines := strings.Split(implContent, "\n")
			if len(lines) > 25 {
				lines = lines[:25]
				lines = append(lines, "...")
			}
			ctx["implementation_excerpt"] = strings.Join(lines, "\n")
		} else {
			// Fallback: gather text from all large content blocks
			var allText []string
			for _, b := range blocks {
				if b.SizeLines >= 4 {
					bc, ok := getBlockContent(string(content), bodyStart, b.Slug)
					if ok && strings.TrimSpace(bc) != "" {
						allText = append(allText, bc)
					}
				}
				if len(allText) >= 5 {
					break
				}
			}
			if len(allText) > 0 {
				combined := strings.Join(allText, "\n")
				lines := strings.Split(combined, "\n")
				if len(lines) > 40 {
					lines = lines[:40]
				}
				ctx["implementation_excerpt"] = strings.Join(lines, "\n")
			}
		}

		// Extract first paragraph of body (title/intro)
		bodyLines := strings.Split(string(content)[bodyStart:], "\n")
		var intro []string
		for _, l := range bodyLines {
			if len(intro) > 0 && strings.TrimSpace(l) == "" {
				break
			}
			if strings.TrimSpace(l) != "" && !strings.HasPrefix(l, "#") {
				intro = append(intro, l)
			}
		}
		if len(intro) > 0 {
			ctx["intro"] = strings.Join(intro, "\n")
		}

		tasks = append(tasks, FixContext{
			ArtifactID: a.ID,
			Path:       a.Path,
			Type:       a.Type,
			Violations: violations,
			Context:    ctx,
		})
	}

	if dispatch {
		outputDispatch(tasks, mode, batchSize, dryRun)
		return
	}

	if planOnly {
		outputFixPlanJSON(tasks, mode)
		return
	}

	if applyStubs {
		applyStubFixes(tasks, dryRun, mode)
		return
	}

	// Default: show plan
	outputFixPlanJSON(tasks, mode)
}

// --- Dispatch output ---

type DispatchBatch struct {
	ID     int           `json:"batch_id"`
	Kind   string        `json:"kind"`
	Prompt string        `json:"prompt"`
	Tasks  []FixContext  `json:"tasks"`
	Stats  DispatchStats `json:"stats"`
}

type DispatchStats struct {
	TaskCount      int `json:"task_count"`
	ViolationCount int `json:"violation_count"`
	ContextChars   int `json:"context_chars"`
	ContextTokens  int `json:"context_tokens_est"`
	PromptTokens   int `json:"prompt_tokens_est"`
}

type DispatchOutput struct {
	Batches    []DispatchBatch `json:"batches"`
	TotalStats DispatchStats   `json:"total"`
}

func outputDispatch(tasks []FixContext, mode OutputMode, batchSize int, dryRun bool) {
	// Group tasks by primary violation kind for optimal batching
	grouped := map[string][]FixContext{}
	kindOrder := []string{"missing", "stub", "format", "bloated"}
	for _, t := range tasks {
		primaryKind := t.Violations[0].Kind
		grouped[primaryKind] = append(grouped[primaryKind], t)
	}

	var batches []DispatchBatch
	batchID := 0

	for _, kind := range kindOrder {
		kindTasks, ok := grouped[kind]
		if !ok {
			continue
		}

		// Split into batches of batchSize
		for i := 0; i < len(kindTasks); i += batchSize {
			end := i + batchSize
			if end > len(kindTasks) {
				end = len(kindTasks)
			}
			chunk := kindTasks[i:end]
			batchID++

			prompt := buildBatchPrompt(kind, chunk)

			// Stats
			violCount := 0
			ctxChars := 0
			for _, t := range chunk {
				violCount += len(t.Violations)
				for _, v := range t.Context {
					ctxChars += len(v)
				}
			}
			promptTokens := len(prompt) / 4

			batches = append(batches, DispatchBatch{
				ID:     batchID,
				Kind:   kind,
				Prompt: prompt,
				Tasks:  chunk,
				Stats: DispatchStats{
					TaskCount:      len(chunk),
					ViolationCount: violCount,
					ContextChars:   ctxChars,
					ContextTokens:  ctxChars / 4,
					PromptTokens:   promptTokens,
				},
			})
		}
	}

	// Total stats
	totalStats := DispatchStats{}
	for _, b := range batches {
		totalStats.TaskCount += b.Stats.TaskCount
		totalStats.ViolationCount += b.Stats.ViolationCount
		totalStats.ContextChars += b.Stats.ContextChars
		totalStats.ContextTokens += b.Stats.ContextTokens
		totalStats.PromptTokens += b.Stats.PromptTokens
	}

	if mode == ModeJSON {
		out := DispatchOutput{Batches: batches, TotalStats: totalStats}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
		return
	}

	// Text mode
	fmt.Printf("dispatch: %d batches, %d tasks, %d violations\n\n", len(batches), totalStats.TaskCount, totalStats.ViolationCount)
	for _, b := range batches {
		fmt.Printf("batch %d [%s] — %d tasks, %d violations, ~%dK prompt tokens\n",
			b.ID, b.Kind, b.Stats.TaskCount, b.Stats.ViolationCount, b.Stats.PromptTokens/1000)
		for _, t := range b.Tasks {
			var parts []string
			for _, v := range t.Violations {
				parts = append(parts, fmt.Sprintf("%s:%s", v.Role, v.Kind))
			}
			fmt.Printf("  %s  %s\n", t.ArtifactID, strings.Join(parts, " "))
		}
		if dryRun && mode == ModeWide {
			fmt.Printf("\n--- PROMPT ---\n%s\n--- END ---\n", b.Prompt)
		}
		fmt.Println()
	}

	fmt.Printf("total: ~%d prompt tokens est\n", totalStats.PromptTokens)
}

func buildBatchPrompt(kind string, tasks []FixContext) string {
	var sb strings.Builder

	// System instruction — minimal, role-specific
	switch kind {
	case "missing":
		sb.WriteString("Fix AWR artifacts by adding missing required blocks.\n")
		sb.WriteString("For each task: read the file path, find where to insert the new section, add it using Edit tool.\n")
		sb.WriteString("Use the pre-extracted context to generate appropriate content.\n")
		sb.WriteString("Each missing block needs a ## heading and meaningful content (not placeholder text).\n")
		sb.WriteString("For insight artifacts: extract from existing content. For methodology: write based on context.\n\n")
	case "stub":
		sb.WriteString("Fix AWR artifacts by expanding stub blocks (too short).\n")
		sb.WriteString("For each task: read the file, find the stub block, expand it with substantive content.\n")
		sb.WriteString("Use the pre-extracted context and implementation excerpt as source material.\n")
		sb.WriteString("Keep the existing content as the opening, add supporting detail below.\n\n")
	case "format":
		sb.WriteString("Fix AWR artifacts with format violations.\n")
		sb.WriteString("For each task: read the file, find the block, restructure content to match expected format.\n")
		sb.WriteString("'list' format = markdown bullet list (- item). 'table' = markdown table. 'prose' = paragraphs.\n")
		sb.WriteString("Preserve all information — only change the structure, not the content.\n\n")
	case "bloated":
		sb.WriteString("Fix AWR artifacts with bloated blocks (too long).\n")
		sb.WriteString("For each task: read the file, find the bloated block, compress it to within the maximum.\n")
		sb.WriteString("Strategies: merge similar items, remove redundancy, extract to subsections, distill.\n")
		sb.WriteString("Preserve key information and actionable content. Remove verbose explanations.\n\n")
	}

	sb.WriteString("RULES:\n")
	sb.WriteString("- Use Edit tool for all changes. One edit per violation.\n")
	sb.WriteString("- Process tasks sequentially: read file → edit → next file.\n")
	sb.WriteString("- Do NOT add comments like 'TODO' or 'TBD'. Write real content.\n")
	sb.WriteString("- Do NOT explain your changes. Just fix and move to the next task.\n")
	sb.WriteString("- Artifact language: use the same language as existing content (Russian or English).\n\n")

	// Task list
	sb.WriteString(fmt.Sprintf("TASKS (%d):\n\n", len(tasks)))

	for i, t := range tasks {
		sb.WriteString(fmt.Sprintf("--- Task %d: %s ---\n", i+1, t.ArtifactID))
		sb.WriteString(fmt.Sprintf("PATH: %s\n", filepath.Join(root, t.Path)))
		sb.WriteString(fmt.Sprintf("TYPE: %s\n", t.Type))

		sb.WriteString("VIOLATIONS:\n")
		for _, v := range t.Violations {
			switch v.Kind {
			case "missing":
				roleTitle := strings.ToUpper(v.Role[:1]) + v.Role[1:]
			sb.WriteString(fmt.Sprintf("  - ADD ## %s (required, currently absent)\n", roleTitle))
			case "stub":
				sb.WriteString(fmt.Sprintf("  - EXPAND ## %s from %dL to >=%sL\n", v.Heading, v.Actual, v.Expected[2:]))
			case "bloated":
				sb.WriteString(fmt.Sprintf("  - COMPRESS ## %s from %dL to <=%sL\n", v.Heading, v.Actual, v.Expected[2:]))
			case "format":
				sb.WriteString(fmt.Sprintf("  - REFORMAT ## %s to %s format\n", v.Heading, v.Expected))
			}
		}

		if len(t.Context) > 0 {
			sb.WriteString("CONTEXT:\n")
			for k, v := range t.Context {
				// Truncate long context
				if len(v) > 500 {
					v = v[:500] + "\n..."
				}
				sb.WriteString(fmt.Sprintf("  [%s]:\n%s\n", k, v))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// --- Plan output ---

func outputFixPlanJSON(tasks []FixContext, mode OutputMode) {
	totalViolations := 0
	for _, t := range tasks {
		totalViolations += len(t.Violations)
	}

	if mode == ModeJSON {
		plan := FixPlanOutput{
			Tasks:    tasks,
			Total:    totalViolations,
			EditHint: "For each task: use Edit tool on the path. Expand/add blocks per violations. Use context fields for source material.",
		}
		data, _ := json.MarshalIndent(plan, "", "  ")
		fmt.Println(string(data))
		return
	}

	// Text mode: compact summary
	fmt.Printf("fix plan: %d tasks, %d violations\n", len(tasks), totalViolations)
	for _, t := range tasks {
		var parts []string
		for _, v := range t.Violations {
			parts = append(parts, fmt.Sprintf("%s:%s", v.Role, v.Kind))
		}
		fmt.Printf("  %s  %s\n", t.ArtifactID, strings.Join(parts, " "))
		if mode == ModeWide {
			for k, v := range t.Context {
				lines := strings.Count(v, "\n") + 1
				fmt.Printf("    ctx.%s: %dL\n", k, lines)
			}
		}
	}
}

// --- Deterministic stub expansion (O1) ---

func applyStubFixes(tasks []FixContext, dryRun bool, mode OutputMode) {
	fixed := 0
	skipped := 0

	for _, task := range tasks {
		for _, v := range task.Violations {
			if v.Kind != "stub" {
				continue
			}

			fpath := filepath.Join(root, task.Path)
			content, err := os.ReadFile(fpath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: cannot read %s: %v\n", task.Path, err)
				skipped++
				continue
			}

			_, bodyStart := parseFrontmatter(string(content))
			blocks := parseBlocks(string(content), bodyStart)

			// Find the stub block
			var stubBlock *Block
			for i := range blocks {
				if blockMatches(blocks[i], v.Role) && blocks[i].Level == 2 {
					stubBlock = &blocks[i]
					break
				}
			}
			if stubBlock == nil {
				skipped++
				continue
			}

			// Find implementation block for source material
			implExcerpt := task.Context["implementation_excerpt"]
			hasRealImpl := false
			// Check if artifact has a real implementation block with content
			bc, found := getBlockContent(string(content), bodyStart, "implementation")
			if found && len(strings.TrimSpace(bc)) > 50 {
				hasRealImpl = true
				if implExcerpt == "" {
					implExcerpt = bc
				}
			}
			if implExcerpt == "" {
				implExcerpt = task.Context["implementation_excerpt"]
			}

			// Generate expanded content deterministically
			expanded := expandStubDeterministic(v.Role, stubBlock, string(content), bodyStart, implExcerpt, hasRealImpl)
			if expanded == "" {
				skipped++
				continue
			}

			if dryRun {
				fmt.Printf("[dry-run] %s #%s: would expand from %dL to %dL\n",
					task.ArtifactID, v.Role, stubBlock.SizeLines, strings.Count(expanded, "\n")+1)
				continue
			}

			// Apply the edit
			fileLines := strings.Split(string(content), "\n")
			startIdx := stubBlock.LineStart - 1 // 0-based
			endIdx := stubBlock.LineEnd          // exclusive
			if startIdx < 0 {
				startIdx = 0
			}
			if endIdx > len(fileLines) {
				endIdx = len(fileLines)
			}

			// Replace stub content with expanded content
			expandedLines := strings.Split(expanded, "\n")
			newLines := make([]string, 0, len(fileLines)-endIdx+startIdx+len(expandedLines))
			newLines = append(newLines, fileLines[:startIdx]...)
			newLines = append(newLines, expandedLines...)
			newLines = append(newLines, fileLines[endIdx:]...)

			newContent := strings.Join(newLines, "\n")
			if err := os.WriteFile(fpath, []byte(newContent), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "error: cannot write %s: %v\n", task.Path, err)
				skipped++
				continue
			}

			fixed++
			if mode != ModeJSON {
				fmt.Printf("fixed %s #%s: %dL → %dL\n",
					task.ArtifactID, v.Role, stubBlock.SizeLines, len(expandedLines))
			}
		}
	}

	if mode == ModeJSON {
		data, _ := json.Marshal(map[string]interface{}{"fixed": fixed, "skipped": skipped})
		fmt.Println(string(data))
	} else if !quietMode {
		fmt.Printf("\n%d fixed, %d skipped\n", fixed, skipped)
	}
}

// expandStubDeterministic generates expanded content for a stub block
// using heuristics: extract key sentences from implementation.
func expandStubDeterministic(role string, stub *Block, content string, bodyStart int, implExcerpt string, hasRealImplementation bool) string {
	if implExcerpt == "" {
		return ""
	}

	// Get current stub content
	stubContent, _ := getBlockContent(content, bodyStart, role)
	stubContent = strings.TrimSpace(stubContent)

	// Extract sentences from implementation
	implSentences := extractSentences(implExcerpt)
	if len(implSentences) < 2 {
		return ""
	}

	switch role {
	case "problem":
		// Only expand if source is from a real implementation block (not fallback)
		if !hasRealImplementation {
			return ""
		}
		return expandProblem(stubContent, implSentences)
	case "solution":
		if !hasRealImplementation {
			return ""
		}
		return expandSolution(stubContent, implSentences)
	case "offer":
		return expandOffer(stubContent, implSentences)
	default:
		return ""
	}
}

// extractSentences splits text into prose sentences, filtering code/tables/noise.
func extractSentences(text string) []string {
	var sentences []string
	inCodeFence := false
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		// Track code fence boundaries
		if strings.HasPrefix(trimmed, "```") {
			inCodeFence = !inCodeFence
			continue
		}
		if inCodeFence {
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") ||
			strings.HasPrefix(trimmed, "|") || strings.HasPrefix(trimmed, "---") ||
			strings.HasPrefix(trimmed, "...") {
			continue
		}
		// Skip tabular data, file paths, JSON, code-like lines
		if strings.Contains(trimmed, "\t") || strings.HasPrefix(trimmed, "{") ||
			strings.HasPrefix(trimmed, "[") {
			continue
		}
		if strings.Contains(trimmed, "/") && !strings.Contains(trimmed, " ") {
			continue
		}
		// Skip lines that are all-caps column headers or short identifiers
		if len(trimmed) < 40 && !strings.ContainsAny(trimmed, ".!?,;:") && strings.ToUpper(trimmed) == trimmed {
			continue
		}
		if len(trimmed) < 30 && !strings.Contains(trimmed, " ") {
			continue
		}
		// Remove list markers
		trimmed = strings.TrimPrefix(trimmed, "- ")
		trimmed = strings.TrimPrefix(trimmed, "* ")
		if len(trimmed) > 2 && trimmed[0] >= '0' && trimmed[0] <= '9' && trimmed[1] == '.' {
			trimmed = strings.TrimSpace(trimmed[2:])
		}

		// Split by sentence-ending punctuation
		parts := splitSentences(trimmed)
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if len(p) > 40 { // Skip short fragments
				sentences = append(sentences, p)
			}
		}
	}
	return sentences
}

func splitSentences(text string) []string {
	var result []string
	current := ""
	for i, r := range text {
		current += string(r)
		if (r == '.' || r == '!' || r == '?') && i < len(text)-1 {
			next := text[i+1]
			if next == ' ' || next == '\n' {
				result = append(result, strings.TrimSpace(current))
				current = ""
			}
		}
	}
	if strings.TrimSpace(current) != "" {
		result = append(result, strings.TrimSpace(current))
	}
	return result
}

func expandProblem(currentStub string, implSentences []string) string {
	// Keep current stub as first line
	lines := []string{currentStub}

	// Add 2-4 supporting sentences from implementation that describe the problem
	// Look for sentences with problem-indicating words
	problemWords := []string{"without", "problem", "fail", "error", "waste", "miss", "lack",
		"no ", "not ", "never", "lose", "broken", "wrong", "bug", "issue",
		"Without", "Problem", "Fail", "Error", "Waste", "Miss", "Lack"}

	added := 0
	for _, s := range implSentences {
		if added >= 3 {
			break
		}
		for _, pw := range problemWords {
			if strings.Contains(s, pw) {
				lines = append(lines, s)
				added++
				break
			}
		}
	}

	// If not enough problem sentences, take first 2 impl sentences as context
	if added < 2 {
		for i, s := range implSentences {
			if added >= 3 || i >= 3 {
				break
			}
			// Check not already added
			found := false
			for _, l := range lines {
				if l == s {
					found = true
					break
				}
			}
			if !found {
				lines = append(lines, s)
				added++
			}
		}
	}

	if len(lines) < 3 {
		return "" // Can't expand enough
	}

	return strings.Join(lines, "\n\n")
}

func expandSolution(currentStub string, implSentences []string) string {
	// Keep current stub as first line
	lines := []string{currentStub}

	// Add 3-6 sentences from implementation describing the approach
	solutionWords := []string{"solve", "approach", "pattern", "mechanism", "step", "phase",
		"create", "implement", "use ", "apply", "define", "build",
		"Solve", "Approach", "Pattern", "Mechanism", "Step", "Phase"}

	added := 0
	for _, s := range implSentences {
		if added >= 5 {
			break
		}
		for _, sw := range solutionWords {
			if strings.Contains(s, sw) {
				lines = append(lines, s)
				added++
				break
			}
		}
	}

	// Fill with first N implementation sentences if needed
	if added < 4 {
		for i, s := range implSentences {
			if added >= 5 || i >= 6 {
				break
			}
			found := false
			for _, l := range lines {
				if l == s {
					found = true
					break
				}
			}
			if !found {
				lines = append(lines, s)
				added++
			}
		}
	}

	if len(lines) < 5 {
		return "" // Can't expand enough
	}

	return strings.Join(lines, "\n\n")
}

func expandOffer(currentStub string, implSentences []string) string {
	lines := []string{currentStub}

	// Add detail sentences from available content
	added := 0
	for _, s := range implSentences {
		if added >= 4 {
			break
		}
		found := false
		for _, l := range lines {
			if l == s {
				found = true
				break
			}
		}
		if !found {
			lines = append(lines, s)
			added++
		}
	}

	if len(lines) < 4 {
		return ""
	}

	return strings.Join(lines, "\n\n")
}
