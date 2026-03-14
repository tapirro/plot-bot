package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Action represents a parsed checklist item from a bet/maintenance file.
type Action struct {
	Index   int    `json:"index"`             // 1-based position within parent
	Text    string `json:"text"`              // action text (without metadata)
	Done    bool   `json:"done"`              // [x] vs [ ]
	Owner   string `json:"owner,omitempty"`   // @owner
	Group   string `json:"group,omitempty"`   // last heading before the item
	Blocked string `json:"blocked,omitempty"` // ~blocked:id
	Tag     string `json:"tag,omitempty"`     // #tag
	Parent  string `json:"parent"`            // parent artifact ID
	PType   string `json:"parent_type"`       // bet or maintenance
	Line    int    `json:"line"`              // line number in file
}

var (
	reCheckbox = regexp.MustCompile(`^- \[([ xX])\] (.+)`)
	reOwner    = regexp.MustCompile(`@(\w[\w-]*)`)
	reBlocked  = regexp.MustCompile(`~blocked:([^\s,]+)`)
	reTag      = regexp.MustCompile(`#(\w[\w-]*)`)
	reIdRef    = regexp.MustCompile(`\([\d.]+(?:,\s*[\d.]+)*\)$`)            // trailing (1.2) or (1.2, 1.3)
	reGroup    = regexp.MustCompile(`^(?:#+\s+)?([A-ZА-Яa-zа-я][\w /&-]+):?\s*$`) // "Core:" or "## Core"
	reBacktick = regexp.MustCompile("`[^`]+`")                              // inline code spans
)

// parseActions reads a file and extracts Action items from checklist lines.
func parseActions(path string, parentID string, parentType string) []Action {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")

	var actions []Action
	idx := 0
	group := ""
	inActions := false

	for lineNum, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect ## Actions or ## Действия heading to start collecting
		if isActionsHeading(trimmed) {
			inActions = true
			group = ""
			continue
		}
		// A new ## section ends the actions block
		if inActions && strings.HasPrefix(trimmed, "## ") && !isActionsHeading(trimmed) {
			inActions = false
			continue
		}

		if !inActions {
			continue
		}

		// Group headers within Actions section (e.g. "Core:", "Long-term:", "## Subsection")
		if gm := reGroup.FindStringSubmatch(trimmed); gm != nil && !strings.HasPrefix(trimmed, "- [") {
			group = strings.TrimSuffix(gm[1], ":")
			continue
		}

		m := reCheckbox.FindStringSubmatch(trimmed)
		if m == nil {
			continue
		}
		idx++
		done := m[1] == "x" || m[1] == "X"
		rawText := m[2]

		// Extract metadata — strip backtick-quoted spans first to avoid false matches
		metaText := reBacktick.ReplaceAllString(rawText, "")
		owner := ""
		blocked := ""
		tag := ""

		if om := reOwner.FindStringSubmatch(metaText); om != nil {
			owner = om[1]
		}
		if bm := reBlocked.FindStringSubmatch(metaText); bm != nil {
			blocked = bm[1]
		}
		if tm := reTag.FindStringSubmatch(metaText); tm != nil {
			tag = tm[1]
		}

		// Clean text: remove only extracted metadata (not backtick-protected)
		cleanText := rawText
		if owner != "" {
			cleanText = strings.Replace(cleanText, "@"+owner, "", 1)
		}
		if blocked != "" {
			cleanText = strings.Replace(cleanText, "~blocked:"+blocked, "", 1)
		}
		if tag != "" {
			cleanText = strings.Replace(cleanText, "#"+tag, "", 1)
		}
		cleanText = reIdRef.ReplaceAllString(cleanText, "")
		cleanText = strings.TrimSpace(cleanText)
		// Clean trailing commas, parens
		cleanText = strings.TrimRight(cleanText, " ,")

		actions = append(actions, Action{
			Index:   idx,
			Text:    cleanText,
			Done:    done,
			Owner:   owner,
			Group:   group,
			Blocked: blocked,
			Tag:     tag,
			Parent:  parentID,
			PType:   parentType,
			Line:    lineNum + 1,
		})
	}
	return actions
}

// allActions collects actions from all bet/maintenance artifacts.
func allActions(idx *Index) []Action {
	var all []Action
	parents := filterArtifacts(idx.Artifacts, func(a Artifact) bool {
		return a.Type == "bet" || a.Type == "maintenance"
	})
	for _, a := range parents {
		path := resolvePath(a.Path)
		actions := parseActions(path, a.ID, a.Type)
		all = append(all, actions...)
	}
	return all
}

func resolvePath(p string) string {
	if strings.HasPrefix(p, "/") {
		return p
	}
	return filepath.Join(root, p)
}

// cmdActions implements the `actions` command.
func cmdActions(args []string, mode OutputMode, idx *Index) error {
	var filterParent, filterOwner, filterStatus string
	var filterBlocked bool

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--parent", "-p":
			i++
			if i < len(args) {
				filterParent = args[i]
			}
		case "--owner", "-o":
			i++
			if i < len(args) {
				filterOwner = args[i]
			}
		case "--status", "-s":
			i++
			if i < len(args) {
				filterStatus = args[i] // "done", "todo", "blocked"
			}
		case "--blocked":
			filterBlocked = true
		}
	}

	actions := allActions(idx)

	// Apply filters
	var filtered []Action
	for _, a := range actions {
		if filterParent != "" && a.Parent != filterParent {
			continue
		}
		if filterOwner != "" && a.Owner != filterOwner {
			continue
		}
		if filterBlocked && a.Blocked == "" {
			continue
		}
		if filterStatus != "" {
			switch filterStatus {
			case "done":
				if !a.Done {
					continue
				}
			case "todo":
				if a.Done || a.Blocked != "" {
					continue
				}
			case "blocked":
				if a.Blocked == "" {
					continue
				}
			}
		}
		filtered = append(filtered, a)
	}

	// Sort: parent, then index
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Parent != filtered[j].Parent {
			return filtered[i].Parent < filtered[j].Parent
		}
		return filtered[i].Index < filtered[j].Index
	})

	if mode == ModeJSON {
		// Summary stats
		total := len(actions)
		done := 0
		blocked := 0
		todo := 0
		for _, a := range actions {
			if a.Done {
				done++
			} else if a.Blocked != "" {
				blocked++
			} else {
				todo++
			}
		}

		out := map[string]interface{}{
			"actions": filtered,
			"stats": map[string]int{
				"total":    total,
				"done":     done,
				"todo":     todo,
				"blocked":  blocked,
				"filtered": len(filtered),
			},
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	// Text output
	currentParent := ""
	for _, a := range filtered {
		if a.Parent != currentParent {
			if currentParent != "" {
				fmt.Println()
			}
			currentParent = a.Parent
			fmt.Printf("── %s ──\n", a.Parent)
		}
		check := "[ ]"
		if a.Done {
			check = "[x]"
		} else if a.Blocked != "" {
			check = "[!]"
		}
		meta := ""
		if a.Owner != "" {
			meta += " @" + a.Owner
		}
		if a.Blocked != "" {
			meta += " ⛔ " + a.Blocked
		}
		if a.Group != "" {
			meta = " [" + a.Group + "]" + meta
		}
		fmt.Printf("  %s %s%s\n", check, a.Text, meta)
	}

	// Stats footer
	total := len(actions)
	done := 0
	blocked := 0
	for _, a := range actions {
		if a.Done {
			done++
		} else if a.Blocked != "" {
			blocked++
		}
	}
	if !quietMode {
		blockedStr := ""
		if blocked > 0 {
			blockedStr = fmt.Sprintf(", %d blocked", blocked)
		}
		fmt.Printf("\n%d actions (%d done, %d todo%s), showing %d\n", total, done, total-done-blocked, blockedStr, len(filtered))
	}
	return nil
}

// cmdDone marks an action as done by toggling [x] in the source file.
func cmdDone(args []string, mode OutputMode, idx *Index) error {
	if len(args) < 2 {
		return &UsageError{Msg: "usage: ask done <parent-id> <action-index>\n       ask done bet-hive-master 3"}
	}

	parentID := args[0]
	var actionIdx int
	fmt.Sscanf(args[1], "%d", &actionIdx)

	if actionIdx < 1 {
		return &UsageError{Msg: "action index must be >= 1"}
	}

	// Find parent artifact
	parent := findByID(idx, parentID)
	if parent == nil {
		return &NotFoundError{What: fmt.Sprintf("artifact %s", parentID)}
	}

	path := resolvePath(parent.Path)
	data, err := os.ReadFile(path)
	if err != nil {
		return &FileError{Path: path, Err: err}
	}

	lines := strings.Split(string(data), "\n")
	checkIdx := 0
	inActions := false
	targetLine := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isActionsHeading(trimmed) {
			inActions = true
			continue
		}
		if inActions && strings.HasPrefix(trimmed, "## ") && !isActionsHeading(trimmed) {
			inActions = false
			continue
		}
		if !inActions {
			continue
		}
		if reCheckbox.MatchString(trimmed) {
			checkIdx++
			if checkIdx == actionIdx {
				targetLine = i
				break
			}
		}
	}

	if targetLine < 0 {
		return &NotFoundError{What: fmt.Sprintf("action #%d in %s", actionIdx, parentID)}
	}

	line := lines[targetLine]
	if strings.Contains(line, "- [x]") || strings.Contains(line, "- [X]") {
		// Toggle off
		line = strings.Replace(line, "- [x]", "- [ ]", 1)
		line = strings.Replace(line, "- [X]", "- [ ]", 1)
	} else {
		line = strings.Replace(line, "- [ ]", "- [x]", 1)
	}
	lines[targetLine] = line

	err = os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
	if err != nil {
		return &FileError{Path: path, Err: err}
	}

	// Parse the action for display
	actions := parseActions(path, parentID, parent.Type)
	if actionIdx <= len(actions) {
		a := actions[actionIdx-1]
		status := "done"
		if !a.Done {
			status = "todo"
		}
		if mode == ModeJSON {
			out := map[string]interface{}{
				"parent": parentID,
				"index":  actionIdx,
				"text":   a.Text,
				"status": status,
			}
			data, _ := json.MarshalIndent(out, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("%s #%d → %s: %s\n", parentID, actionIdx, status, a.Text)
		}
	}
	return nil
}

// cmdAssign sets or changes the @owner on an action line.
func cmdAssign(args []string, mode OutputMode, idx *Index) error {
	if len(args) < 3 {
		return &UsageError{Msg: "usage: ask assign <parent-id> <action-index> <owner>\n       ask assign bet-hive-master 3 vadim"}
	}

	parentID := args[0]
	var actionIdx int
	fmt.Sscanf(args[1], "%d", &actionIdx)
	newOwner := strings.TrimPrefix(args[2], "@")

	if newOwner == "" {
		return &UsageError{Msg: "owner cannot be empty"}
	}

	if actionIdx < 1 {
		return &UsageError{Msg: "action index must be >= 1"}
	}

	parent := findByID(idx, parentID)
	if parent == nil {
		return &NotFoundError{What: fmt.Sprintf("artifact %s", parentID)}
	}

	path := resolvePath(parent.Path)
	data, err := os.ReadFile(path)
	if err != nil {
		return &FileError{Path: path, Err: err}
	}

	lines := strings.Split(string(data), "\n")
	checkIdx := 0
	inActions := false
	targetLine := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isActionsHeading(trimmed) {
			inActions = true
			continue
		}
		if inActions && strings.HasPrefix(trimmed, "## ") && !isActionsHeading(trimmed) {
			inActions = false
			continue
		}
		if !inActions {
			continue
		}
		if reCheckbox.MatchString(trimmed) {
			checkIdx++
			if checkIdx == actionIdx {
				targetLine = i
				break
			}
		}
	}

	if targetLine < 0 {
		return &NotFoundError{What: fmt.Sprintf("action #%d in %s", actionIdx, parentID)}
	}

	line := lines[targetLine]
	// Remove existing @owner (outside backticks)
	if om := reOwner.FindString(reBacktick.ReplaceAllString(line, "")); om != "" {
		line = strings.Replace(line, " "+om, "", 1)
		line = strings.Replace(line, om+" ", "", 1)
		line = strings.Replace(line, om, "", 1)
	}
	// Append new @owner before any trailing parenthetical refs
	line = strings.TrimRight(line, " ")
	if loc := reIdRef.FindStringIndex(line); loc != nil {
		line = line[:loc[0]] + "@" + newOwner + " " + line[loc[0]:]
	} else {
		line = line + " @" + newOwner
	}
	lines[targetLine] = line

	err = os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
	if err != nil {
		return &FileError{Path: path, Err: err}
	}

	actions := parseActions(path, parentID, parent.Type)
	if actionIdx <= len(actions) {
		a := actions[actionIdx-1]
		if mode == ModeJSON {
			out := map[string]interface{}{
				"parent": parentID,
				"index":  actionIdx,
				"text":   a.Text,
				"owner":  newOwner,
			}
			data, _ := json.MarshalIndent(out, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("%s #%d → @%s: %s\n", parentID, actionIdx, newOwner, a.Text)
		}
	}
	return nil
}

// cmdStatus changes the status: field in an artifact's frontmatter.
func cmdStatus(args []string, mode OutputMode, idx *Index) error {
	if len(args) < 2 {
		return &UsageError{Msg: "usage: ask status <artifact-id> <new-status>\n       ask status bet-hive-master active"}
	}

	artifactID := args[0]
	newStatus := args[1]

	artifact := findByID(idx, artifactID)
	if artifact == nil {
		return &NotFoundError{What: fmt.Sprintf("artifact %s", artifactID)}
	}

	path := resolvePath(artifact.Path)
	data, err := os.ReadFile(path)
	if err != nil {
		return &FileError{Path: path, Err: err}
	}

	lines := strings.Split(string(data), "\n")
	inFrontmatter := false
	statusLine := -1
	oldStatus := ""

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			break // end of frontmatter
		}
		if inFrontmatter && strings.HasPrefix(trimmed, "status:") {
			oldStatus = strings.TrimSpace(strings.TrimPrefix(trimmed, "status:"))
			statusLine = i
			break
		}
	}

	if statusLine < 0 {
		return &NotFoundError{What: fmt.Sprintf("status field in frontmatter of %s", artifactID)}
	}

	lines[statusLine] = "status: " + newStatus

	err = os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
	if err != nil {
		return &FileError{Path: path, Err: err}
	}

	if mode == ModeJSON {
		out := map[string]interface{}{
			"id":         artifactID,
			"old_status": oldStatus,
			"new_status": newStatus,
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("%s: %s → %s\n", artifactID, oldStatus, newStatus)
	}
	return nil
}

// cmdProgress shows domain-level action progress (done/total per domain).
func cmdProgress(args []string, mode OutputMode, idx *Index) {
	all := allActions(idx)

	// Build domain map from parent artifact
	parentDomain := map[string]string{}
	for _, a := range idx.Artifacts {
		if a.Domain != "" {
			parentDomain[a.ID] = a.Domain
		}
	}

	type domProgress struct {
		Domain  string `json:"domain"`
		Total   int    `json:"total"`
		Done    int    `json:"done"`
		Open    int    `json:"open"`
		Blocked int    `json:"blocked"`
		Pct     int    `json:"pct"`
	}

	byDomain := map[string]*domProgress{}
	for _, a := range all {
		dom := parentDomain[a.Parent]
		if dom == "" {
			dom = "_untagged"
		}
		// Roll up to top-level domain
		parts := strings.SplitN(dom, "/", 2)
		topDom := parts[0]

		dp, ok := byDomain[topDom]
		if !ok {
			dp = &domProgress{Domain: topDom}
			byDomain[topDom] = dp
		}
		dp.Total++
		if a.Done {
			dp.Done++
		} else if a.Blocked != "" {
			dp.Blocked++
		} else {
			dp.Open++
		}
	}

	// Sort by total descending
	var sorted []*domProgress
	for _, dp := range byDomain {
		if dp.Total > 0 {
			dp.Pct = dp.Done * 100 / dp.Total
		}
		sorted = append(sorted, dp)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Total > sorted[j].Total
	})

	if mode == ModeJSON {
		data, _ := json.MarshalIndent(sorted, "", "  ")
		fmt.Println(string(data))
		return
	}

	t := (&Table{}).Col("domain", 20).Col("total", 5).Col("done", 5).Col("open", 5).Col("blocked", 7).Col("pct", 5).Col("bar", 20)
	totalAll, doneAll := 0, 0
	for _, dp := range sorted {
		bar := ""
		if dp.Total > 0 {
			filled := dp.Pct / 5
			bar = strings.Repeat("█", filled) + strings.Repeat("░", 20-filled)
		}
		t.Row(dp.Domain, fmt.Sprintf("%d", dp.Total), fmt.Sprintf("%d", dp.Done),
			fmt.Sprintf("%d", dp.Open), fmt.Sprintf("%d", dp.Blocked),
			fmt.Sprintf("%d%%", dp.Pct), bar)
		totalAll += dp.Total
		doneAll += dp.Done
	}
	t.Render(ModeWide)
	fmt.Println(strings.Repeat("─", 65))
	overallPct := 0
	if totalAll > 0 {
		overallPct = doneAll * 100 / totalAll
	}
	fmt.Printf("%-20s %5d %5d %5s %7s %4d%%\n", "TOTAL", totalAll, doneAll, "", "", overallPct)
}

// betInfo holds enriched bet data for @bets view.
type betInfo struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Status     string   `json:"status"`
	Domain     string   `json:"domain"`
	Horizon    string   `json:"horizon,omitempty"`
	Meanings   []string `json:"meanings,omitempty"`
	Tension    string   `json:"tension,omitempty"`
	Hypothesis string   `json:"hypothesis,omitempty"`
	Total      int      `json:"actions_total"`
	Done       int      `json:"actions_done"`
	Blocked    int      `json:"actions_blocked"`
	Pct        int      `json:"pct"`
	// Telema2 live data (from cache)
	TelemaID          string   `json:"telema_id,omitempty"`
	TasksTotal        int      `json:"tasks_total,omitempty"`
	TasksDone         int      `json:"tasks_done,omitempty"`
	MeasurementActual *float64 `json:"measurement_actual,omitempty"`
	MeasurementTarget *float64 `json:"measurement_target,omitempty"`
}

// loadBetInfo reads frontmatter extras and action counts for a bet artifact.
func loadBetInfo(a *Artifact) betInfo {
	bi := betInfo{
		ID:     a.ID,
		Title:  a.Title,
		Status: a.Status,
		Domain: a.Domain,
	}

	path := resolvePath(a.Path)
	data, err := os.ReadFile(path)
	if err != nil {
		return bi
	}

	fm, _ := parseFrontmatter(string(data))
	if fm != nil {
		if v, ok := fm["horizon"]; ok {
			bi.Horizon = fmt.Sprintf("%v", v)
		}
		if v, ok := fm["tension"]; ok {
			bi.Tension = fmt.Sprintf("%v", v)
		}
		if v, ok := fm["hypothesis"]; ok {
			bi.Hypothesis = fmt.Sprintf("%v", v)
		}
		if v, ok := fm["meanings"]; ok {
			if arr, ok := v.([]string); ok {
				bi.Meanings = arr
			}
		}
	}

	actions := parseActions(path, a.ID, a.Type)
	for _, act := range actions {
		bi.Total++
		if act.Done {
			bi.Done++
		} else if act.Blocked != "" {
			bi.Blocked++
		}
	}
	if bi.Total > 0 {
		bi.Pct = bi.Done * 100 / bi.Total
	}

	return bi
}

// cmdBets shows a strategic overview of all bets with action progress.
func cmdBets(args []string, mode OutputMode, idx *Index) {
	var bets []betInfo
	hasCache := loadCache() != nil
	for i := range idx.Artifacts {
		if idx.Artifacts[i].Type != "bet" {
			continue
		}
		// Skip template bets
		if strings.Contains(idx.Artifacts[i].ID, "example-template") {
			continue
		}
		bi := loadBetInfo(&idx.Artifacts[i])
		if hasCache {
			enrichBetInfo(&bi)
		}
		bets = append(bets, bi)
	}

	// Group by status
	statusOrder := []string{"experiment", "proposed", "validated", "scaled", "failed"}
	byStatus := map[string][]betInfo{}
	for _, b := range bets {
		byStatus[b.Status] = append(byStatus[b.Status], b)
	}

	// Sort within each group: by pct descending
	for _, group := range byStatus {
		sort.Slice(group, func(i, j int) bool {
			return group[i].Pct > group[j].Pct
		})
	}

	if mode == ModeJSON {
		out := map[string]interface{}{
			"bets":  bets,
			"total": len(bets),
		}
		// Summary
		totalActions, doneActions := 0, 0
		for _, b := range bets {
			totalActions += b.Total
			doneActions += b.Done
		}
		out["actions_total"] = totalActions
		out["actions_done"] = doneActions
		if totalActions > 0 {
			out["actions_pct"] = doneActions * 100 / totalActions
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
		return
	}

	totalActions, doneActions := 0, 0
	for _, b := range bets {
		totalActions += b.Total
		doneActions += b.Done
	}
	overallPct := 0
	if totalActions > 0 {
		overallPct = doneActions * 100 / totalActions
	}
	fmt.Printf("BETS — %d active · %d%% actions done (%d/%d)\n\n", len(bets), overallPct, doneActions, totalActions)

	for _, status := range statusOrder {
		group := byStatus[status]
		if len(group) == 0 {
			continue
		}
		fmt.Printf("%s (%d)\n", status, len(group))
		for _, b := range group {
			bar := ""
			if b.Total > 0 {
				filled := b.Pct * 10 / 100
				if filled > 10 {
					filled = 10
				}
				bar = strings.Repeat("▓", filled) + strings.Repeat("░", 10-filled)
			} else {
				bar = "          "
			}
			meanings := ""
			if len(b.Meanings) > 0 {
				meanings = " [" + strings.Join(b.Meanings, ",") + "]"
			}
			horizon := ""
			if b.Horizon != "" {
				horizon = " →" + b.Horizon
			}
			tasks := ""
		if b.TasksTotal > 0 {
			tasks = fmt.Sprintf(" T:%d/%d", b.TasksDone, b.TasksTotal)
		}
		measure := ""
		if b.MeasurementActual != nil && b.MeasurementTarget != nil {
			measure = fmt.Sprintf(" M:%.0f/%.0f", *b.MeasurementActual, *b.MeasurementTarget)
		}
		fmt.Printf("  %s %3d%% %d/%d  %-30s %s%s%s%s%s\n",
				bar, b.Pct, b.Done, b.Total, b.Title, b.Domain, meanings, horizon, tasks, measure)
		}
		fmt.Println()
	}

	if hasCache {
		age := cacheAge()
		ageStr := "?"
		if age >= 0 {
			if age < time.Hour {
				ageStr = fmt.Sprintf("%dm", int(age.Minutes()))
			} else {
				ageStr = fmt.Sprintf("%dh", int(age.Hours()))
			}
		}
		linked := 0
		for _, b := range bets {
			if b.TelemaID != "" {
				linked++
			}
		}
		fmt.Printf("cache: %s ago · %d/%d linked to Telema2\n", ageStr, linked, len(bets))
	}
}

// cmdTensions extracts tensions from all bet frontmatters.
// getDeliveryGapTensions generates tensions from value chain nodes without deliverables.
// Uses vchain.py json (full graph) to get unmapped nodes with bottleneck info in one call.
func getDeliveryGapTensions() []tensionInfo {
	vchainPath := filepath.Join(root, "tools", "scripts", "vchain.py")
	cmd := exec.Command("python3", vchainPath, "json")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var graph map[string]interface{}
	if json.Unmarshal(out, &graph) != nil {
		return nil
	}

	// Collect node IDs that have deliverables
	coveredNodes := map[string]bool{}
	companies, _ := graph["companies"].([]interface{})
	for _, co := range companies {
		comp, _ := co.(map[string]interface{})
		nodes, _ := comp["nodes"].([]interface{})
		var walkNodes func([]interface{})
		walkNodes = func(nodeList []interface{}) {
			for _, n := range nodeList {
				nd, _ := n.(map[string]interface{})
				delivs, _ := nd["deliverables"].([]interface{})
				if len(delivs) > 0 {
					if nid, ok := nd["id"].(string); ok {
						coveredNodes[nid] = true
					}
				}
				children, _ := nd["children"].([]interface{})
				if len(children) > 0 {
					walkNodes(children)
				}
			}
		}
		walkNodes(nodes)
	}

	// Walk all nodes, find uncovered bottleneck/manual/poc nodes
	var tensions []tensionInfo
	for _, co := range companies {
		comp, _ := co.(map[string]interface{})
		nodes, _ := comp["nodes"].([]interface{})
		var walkForGaps func([]interface{})
		walkForGaps = func(nodeList []interface{}) {
			for _, n := range nodeList {
				nd, _ := n.(map[string]interface{})
				nid, _ := nd["id"].(string)
				if nid == "" || coveredNodes[nid] {
					children, _ := nd["children"].([]interface{})
					if len(children) > 0 {
						walkForGaps(children)
					}
					continue
				}

				isBottleneck := false
				if b, ok := nd["bottleneck"].(bool); ok && b {
					isBottleneck = true
				}
				maturity, _ := nd["maturity"].(string)
				name, _ := nd["name"].(string)
				gaps, _ := nd["gaps"].([]interface{})

				if !isBottleneck && maturity != "manual" && maturity != "poc" {
					children, _ := nd["children"].([]interface{})
					if len(children) > 0 {
						walkForGaps(children)
					}
					continue
				}

				tension := fmt.Sprintf("No deliverables targeting %s (maturity: %s", nid, maturity)
				if len(gaps) > 0 {
					tension += fmt.Sprintf(", %d gaps", len(gaps))
				}
				tension += ")"

				hyp := ""
				if isBottleneck {
					hyp = fmt.Sprintf("Bottleneck node — deliverable here would unblock %s flow", name)
				}

				domain := nid
				if si := strings.Index(nid, "/"); si > 0 {
					domain = nid[:si]
				}

				tensions = append(tensions, tensionInfo{
					BetID:      "delivery:" + nid,
					BetTitle:   fmt.Sprintf("Delivery gap: %s", name),
					Status:     "delivery-gap",
					Domain:     domain,
					Tension:    tension,
					Hypothesis: hyp,
					Pct:        0,
				})

				children, _ := nd["children"].([]interface{})
				if len(children) > 0 {
					walkForGaps(children)
				}
			}
		}
		walkForGaps(nodes)
	}
	return tensions
}

type tensionInfo struct {
	BetID      string `json:"bet_id"`
	BetTitle   string `json:"bet_title"`
	Status     string `json:"status"`
	Domain     string `json:"domain"`
	Tension    string `json:"tension"`
	Hypothesis string `json:"hypothesis,omitempty"`
	Pct        int    `json:"pct"`
}

func cmdTensions(args []string, mode OutputMode, idx *Index) {
	var tensions []tensionInfo
	for i := range idx.Artifacts {
		a := &idx.Artifacts[i]
		if a.Type != "bet" {
			continue
		}
		if strings.Contains(a.ID, "example-template") {
			continue
		}
		path := resolvePath(a.Path)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		fm, _ := parseFrontmatter(string(data))
		if fm == nil {
			continue
		}
		t, ok := fm["tension"]
		if !ok || fmt.Sprintf("%v", t) == "" {
			continue
		}

		// Count action progress
		actions := parseActions(path, a.ID, a.Type)
		total, done := len(actions), 0
		for _, act := range actions {
			if act.Done {
				done++
			}
		}
		pct := 0
		if total > 0 {
			pct = done * 100 / total
		}

		hyp := ""
		if h, ok := fm["hypothesis"]; ok {
			hyp = fmt.Sprintf("%v", h)
		}

		tensions = append(tensions, tensionInfo{
			BetID:      a.ID,
			BetTitle:   a.Title,
			Status:     a.Status,
			Domain:     a.Domain,
			Tension:    fmt.Sprintf("%v", t),
			Hypothesis: hyp,
			Pct:        pct,
		})
	}

	// Add delivery gap tensions
	delivGaps := getDeliveryGapTensions()
	tensions = append(tensions, delivGaps...)

	// Sort: experiment first, then delivery gaps, then by domain
	sort.Slice(tensions, func(i, j int) bool {
		if tensions[i].Status != tensions[j].Status {
			if tensions[i].Status == "experiment" {
				return true
			}
			if tensions[j].Status == "experiment" {
				return false
			}
			if tensions[i].Status == "delivery-gap" {
				return tensions[j].Status != "experiment"
			}
		}
		return tensions[i].Domain < tensions[j].Domain
	})

	if mode == ModeJSON {
		data, _ := json.MarshalIndent(tensions, "", "  ")
		fmt.Println(string(data))
		return
	}

	betCount := 0
	for _, t := range tensions {
		if t.Status != "delivery-gap" {
			betCount++
		}
	}
	fmt.Printf("TENSIONS — %d from bets, %d from delivery gaps\n\n", betCount, len(delivGaps))
	for _, t := range tensions {
		statusMark := "○"
		if t.Status == "experiment" {
			statusMark = "●"
		} else if t.Status == "delivery-gap" {
			statusMark = "△"
		}
		fmt.Printf("%s %-12s %3d%%  %s\n", statusMark, t.Domain, t.Pct, t.BetTitle)
		fmt.Printf("  ⚡ %s\n", t.Tension)
		if t.Hypothesis != "" {
			fmt.Printf("  → %s\n", t.Hypothesis)
		}
		fmt.Println()
	}
}
