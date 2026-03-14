package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// --- Quiet mode ---

var quietMode bool

// --- Composite commands ---

// cmdHealth: single-command workshop health check (replaces scan + lint + gaps)
func cmdHealth(args []string, mode OutputMode) {
	idx := buildIndex()
	if err := saveIndex(idx); err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot save index: %v\n", err)
		os.Exit(1)
	}

	// Parse --domain flag
	domainFilter := ""
	for i, a := range args {
		if a == "--domain" && i+1 < len(args) {
			domainFilter = args[i+1]
			break
		}
	}

	items := idx.Artifacts
	if domainFilter != "" {
		items = filterArtifacts(items, func(a Artifact) bool {
			return strings.EqualFold(a.Domain, domainFilter) ||
				strings.HasPrefix(strings.ToLower(a.Domain), strings.ToLower(domainFilter)+"/")
		})
	}
	meet := 0
	errCount := 0
	var gaps []Artifact
	var stale []Artifact
	now := time.Now()

	for _, a := range items {
		req := requiredTierFor(a)
		if a.Tier >= req {
			meet++
		} else {
			gaps = append(gaps, a)
		}
		// Count lint ERRs (invalid status)
		if vs, ok := validStatuses[a.Type]; ok {
			found := false
			for _, v := range vs {
				if strings.EqualFold(a.Status, v) {
					found = true
					break
				}
			}
			if !found {
				errCount++
			}
		}
		// Stale check
		d, err := time.Parse("2006-01-02", a.Updated)
		if err != nil {
			continue
		}
		age := int(now.Sub(d).Hours() / 24)
		if (a.Status == "draft" && age > 14) || (a.Status == "imported" && age > 90) || age > 180 {
			stale = append(stale, a)
		}
	}

	// Provenance gaps: types requiring confidence but missing it
	provenanceGaps := 0
	for _, a := range items {
		if confidenceRequired[a.Type] && a.Confidence == "" {
			provenanceGaps++
		}
	}

	pct := compliancePct(meet, len(items))

	// Chain coverage: scan deliverables for chain_nodes
	delivTotal, delivShipped, delivNodes := scanDeliverableCoverage()

	if mode == ModeJSON {
		result := map[string]interface{}{
			"total":          len(items),
			"compliance":     pct,
			"gaps":           len(gaps),
			"errors":         errCount,
			"stale":          len(stale),
			"provenance_gaps": provenanceGaps,
			"deliverables": map[string]int{
				"total":   delivTotal,
				"shipped": delivShipped,
				"nodes":   delivNodes,
			},
		}
		if domainFilter != "" {
			result["domain"] = domainFilter
		}
		if len(gaps) > 0 {
			var gapList []map[string]string
			for _, g := range gaps {
				gapList = append(gapList, map[string]string{
					"id": g.ID, "type": g.Type, "path": g.Path,
				})
			}
			result["gap_list"] = gapList
		}
		jsonPrintCompact(result)
		return
	}

	fmt.Printf("%d artifacts, compliance:%d%%, %d ERR", len(items), pct, errCount)
	if provenanceGaps > 0 {
		fmt.Printf(", provenance(%d)", provenanceGaps)
	}
	fmt.Println()
	if len(gaps) > 0 {
		fmt.Printf("gaps(%d):", len(gaps))
		for i, g := range gaps {
			if i > 0 {
				fmt.Print(",")
			}
			fmt.Printf(" %s", g.ID)
		}
		fmt.Println()
	}
	if len(stale) > 0 {
		// Sort stale by age (oldest first)
		sort.Slice(stale, func(i, j int) bool {
			return stale[i].Updated < stale[j].Updated
		})
		limit := 5
		if len(stale) < limit {
			limit = len(stale)
		}
		fmt.Printf("stale(%d):", len(stale))
		for i := 0; i < limit; i++ {
			d, _ := time.Parse("2006-01-02", stale[i].Updated)
			age := int(now.Sub(d).Hours() / 24)
			if i > 0 {
				fmt.Print(",")
			}
			fmt.Printf(" %s(%dd)", stale[i].ID, age)
		}
		if len(stale) > limit {
			fmt.Printf(" +%d more", len(stale)-limit)
		}
		fmt.Println()
	}
	if delivTotal > 0 {
		// Count total value chain nodes for coverage %
		totalVCNodes := countValueChainNodes()
		if totalVCNodes > 0 {
			covPct := delivNodes * 100 / totalVCNodes
			fmt.Printf("delivery: %d deliverables (%d shipped), %d/%d nodes (%d%%)\n", delivTotal, delivShipped, delivNodes, totalVCNodes, covPct)
		} else {
			fmt.Printf("delivery: %d deliverables (%d shipped), %d nodes covered\n", delivTotal, delivShipped, delivNodes)
		}
	}
	if !quietMode && pct < 100 {
		fmt.Printf("→ next: ./ask lint --gap\n")
	}
}

// cmdFind: fuzzy search + instant context (replaces list → get --meta)
func cmdFind(args []string, mode OutputMode, idx *Index) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: ask find <query>")
		os.Exit(1)
	}

	query := strings.Join(args, " ")
	match := resolveArtifact(query, idx.Artifacts)

	if match == nil {
		fmt.Fprintln(os.Stderr, "0 matches")
		os.Exit(1)
	}

	if matches, ok := match.([]Artifact); ok {
		if mode == ModeJSON {
			var items []map[string]interface{}
			for _, a := range matches {
				items = append(items, map[string]interface{}{
					"id": a.ID, "type": a.Type, "status": a.Status, "path": a.Path,
				})
			}
			jsonPrintCompact(map[string]interface{}{"matches": items, "total": len(items)})
		} else {
			fmt.Fprintf(os.Stderr, "%d matches:\n", len(matches))
			for _, a := range matches {
				fmt.Printf("  %s\t%s\t%s\n", a.ID, a.Type, a.Status)
			}
		}
		return
	}

	a := match.(Artifact)

	// Read file to get blocks
	content, err := os.ReadFile(filepath.Join(root, a.Path))
	var blocks []Block
	if err == nil {
		_, bodyStart := parseFrontmatter(string(content))
		blocks = parseBlocks(string(content), bodyStart)
	}

	if mode == ModeJSON {
		result := map[string]interface{}{
			"id": a.ID, "type": a.Type, "status": a.Status,
			"path": a.Path, "tier": a.Tier, "tags": a.Tags,
		}
		if len(blocks) > 0 {
			var blist []map[string]interface{}
			for _, b := range blocks {
				if b.Level == 2 { // Only ## blocks
					blist = append(blist, map[string]interface{}{
						"role": b.Role, "slug": b.Slug, "size": b.SizeLines,
					})
				}
			}
			result["blocks"] = blist
		}
		jsonPrintCompact(result)
		return
	}

	// Compact one-shot info
	fmt.Printf("%s\t%s\n", a.ID, a.Path)
	fmt.Printf("type:%s status:%s tier:%d", a.Type, a.Status, a.Tier)
	if len(a.Tags) > 0 {
		fmt.Printf(" tags:%s", strings.Join(a.Tags, ","))
	}
	fmt.Println()

	if len(blocks) > 0 {
		var blockSummary []string
		for _, b := range blocks {
			if b.Level == 2 {
				name := b.Slug
				if b.Role != "" {
					name = b.Role
				}
				blockSummary = append(blockSummary, fmt.Sprintf("%s(%dL)", name, b.SizeLines))
			}
		}
		fmt.Printf("blocks(%d): %s\n", len(blockSummary), strings.Join(blockSummary, " "))
	}

	// Smart hint based on type
	if !quietMode {
		if len(blocks) > 0 {
			// Find smallest useful block
			var best Block
			for _, b := range blocks {
				if b.Level == 2 && b.SizeLines > 0 {
					if best.Slug == "" || b.SizeLines < best.SizeLines {
						best = b
					}
				}
			}
			addr := best.Slug
			if best.Role != "" {
				addr = best.Role
			}
			if addr != "" {
				fmt.Printf("→ next: ./ask g %s#%s\n", a.ID, addr)
			}
		}
	}
}

// cmdDigest: extract role blocks from recent insights
func cmdDigest(args []string, mode OutputMode, idx *Index) {
	roles := []string{"decisions", "actions"}
	pastFilter := ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--role", "-r":
			i++
			if i < len(args) {
				roles = strings.Split(args[i], ",")
			}
		case "--past", "-p":
			i++
			if i < len(args) {
				pastFilter = args[i]
			}
		}
	}

	// Filter insights via pipeline
	insights := Query(idx.Artifacts).
		Type("insight").
		Past(pastFilter).
		SortDefault().
		Items()

	if len(insights) == 0 {
		if mode == ModeJSON {
			fmt.Println(`{"items":[],"total":0}`)
		} else {
			fmt.Println("0 insights found")
		}
		return
	}

	if mode == ModeJSON {
		var results []map[string]interface{}
		for _, a := range insights {
			content, err := os.ReadFile(filepath.Join(root, a.Path))
			if err != nil {
				continue
			}
			_, bodyStart := parseFrontmatter(string(content))
			blocks := make(map[string]string)
			for _, role := range roles {
				bc, found := getBlockContent(string(content), bodyStart, role)
				if found {
					blocks[role] = bc
				}
			}
			if len(blocks) > 0 {
				results = append(results, map[string]interface{}{
					"id": a.ID, "title": a.Title, "blocks": blocks,
				})
			}
		}
		jsonPrintCompact(map[string]interface{}{"items": results, "total": len(results)})
		return
	}

	for i, a := range insights {
		if i > 0 {
			fmt.Println("---")
		}
		content, err := os.ReadFile(filepath.Join(root, a.Path))
		if err != nil {
			continue
		}
		_, bodyStart := parseFrontmatter(string(content))
		roleQuery := strings.Join(roles, ",")
		combined := getMultiBlockContent(string(content), bodyStart, strings.Split(roleQuery, ","))
		if combined != "" {
			fmt.Printf("=== %s ===\n", a.ID)
			fmt.Println(combined)
		}
	}
}

// --- Views ---

type View struct {
	Name        string
	Description string
	Run         func(args []string, mode OutputMode, idx *Index)
}

var views = map[string]View{
	"health": {
		Name: "health", Description: "workshop health: compliance, gaps, stale, errors",
		Run: func(args []string, mode OutputMode, idx *Index) {
			cmdHealth(args, mode)
		},
	},
	"stale": {
		Name: "stale", Description: "stale artifacts (drafts >14d, imported >90d, any >180d)",
		Run: func(args []string, mode OutputMode, idx *Index) {
			cmdList(append([]string{"--old"}, args...), mode, idx)
		},
	},
	"recent": {
		Name: "recent", Description: "10 most recently updated artifacts",
		Run: func(args []string, mode OutputMode, idx *Index) {
			cmdList(append([]string{"--limit", "10"}, args...), mode, idx)
		},
	},
	"tier0": {
		Name: "tier0", Description: "artifacts with no metadata (tier 0)",
		Run: func(args []string, mode OutputMode, idx *Index) {
			cmdList(append([]string{"--where", "tier=0", "--fields", "id,type,path"}, args...), mode, idx)
		},
	},
	"orphans": {
		Name: "orphans", Description: "artifacts with no incoming or outgoing links",
		Run: func(args []string, mode OutputMode, idx *Index) {
			cmdMap(append([]string{"--orphans"}, args...), mode, idx)
		},
	},
	"patterns": {
		Name: "patterns", Description: "all patterns with title and status",
		Run: func(args []string, mode OutputMode, idx *Index) {
			cmdList(append([]string{"--type", "pattern", "--fields", "id,title,status"}, args...), mode, idx)
		},
	},
	"decisions": {
		Name: "decisions", Description: "extract decisions+actions from recent insights",
		Run: func(args []string, mode OutputMode, idx *Index) {
			cmdDigest(args, mode, idx)
		},
	},
	"audit": {
		Name: "audit", Description: "block contracts audit + fix plan",
		Run: func(args []string, mode OutputMode, idx *Index) {
			cmdAudit(args, mode, idx)
		},
	},
	"actions": {
		Name: "actions", Description: "all inline actions from bets+maintenance",
		Run: func(args []string, mode OutputMode, idx *Index) {
			cmdActions(args, mode, idx)
		},
	},
	"todo": {
		Name: "todo", Description: "open actions (not done, not blocked)",
		Run: func(args []string, mode OutputMode, idx *Index) {
			cmdActions(append([]string{"--status", "todo"}, args...), mode, idx)
		},
	},
	"tree": {
		Name: "tree", Description: "domain tree with artifact counts",
		Run: func(args []string, mode OutputMode, idx *Index) {
			cmdDomainTree(args, mode, idx)
		},
	},
	"progress": {
		Name: "progress", Description: "action progress by domain (done/total/blocked)",
		Run: func(args []string, mode OutputMode, idx *Index) {
			cmdProgress(args, mode, idx)
		},
	},
	"bets": {
		Name: "bets", Description: "strategic overview of all bets with action progress",
		Run: func(args []string, mode OutputMode, idx *Index) {
			cmdBets(args, mode, idx)
		},
	},
	"tensions": {
		Name: "tensions", Description: "tensions driving bets (from frontmatter)",
		Run: func(args []string, mode OutputMode, idx *Index) {
			cmdTensions(args, mode, idx)
		},
	},
	"telema": {
		Name: "telema", Description: "Telema2 cache status + task overview",
		Run: func(args []string, mode OutputMode, idx *Index) {
			cmdTelema(args, mode, idx)
		},
	},
	"delivery": {
		Name: "delivery", Description: "deliverable → chain node coverage",
		Run: func(args []string, mode OutputMode, idx *Index) {
			cmdDelivery(args, mode, idx)
		},
	},
	"contract": {
		Name: "contract", Description: "agent slot contract (computed from repo state)",
		Run: func(args []string, mode OutputMode, idx *Index) {
			cmdContract(args, mode, idx)
		},
	},
	"provenance": {
		Name: "provenance", Description: "artifacts requiring confidence but missing it",
		Run: func(args []string, mode OutputMode, idx *Index) {
			cmdList(append([]string{"--type", "pattern,methodology,best-practice", "--where", "confidence=", "--fields", "id,type,confidence,origin"}, args...), mode, idx)
		},
	},
	"deliveries": {
		Name: "deliveries", Description: "delivery log (pending and recent)",
		Run: func(args []string, mode OutputMode, idx *Index) {
			cmdDeliveries(args, mode, idx)
		},
	},
}

func barStr(count, maxLen int) string {
	filled := count
	if filled > maxLen {
		filled = maxLen
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", maxLen-filled)
}

func runView(name string, args []string, mode OutputMode) bool {
	v, ok := views[name]
	if !ok {
		return false
	}
	idx := loadIndex()
	v.Run(args, mode, idx)
	return true
}

func listViews(mode OutputMode) {
	if mode == ModeJSON {
		var items []map[string]string
		for name, v := range views {
			items = append(items, map[string]string{"name": name, "description": v.Description})
		}
		jsonPrintCompact(map[string]interface{}{"views": items})
		return
	}
	fmt.Println("views:")
	sorted := make([]string, 0, len(views))
	for name := range views {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	for _, name := range sorted {
		fmt.Printf("  @%-12s %s\n", name, views[name].Description)
	}
}
