package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// --- list ---

func cmdList(args []string, mode OutputMode, idx *Index) {
	items := idx.Artifacts

	var typeFilter, tagFilter, statusFilter, zoneFilter, pastFilter, confidenceFilter, originFilter string
	var whereExpr, contentMatch, hasSection, fieldsSpec, sortField, groupByField string
	var oldFilter, gapFilter, countOnly bool
	tierFilter := -1
	limit := 0
	var positional string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--type", "-t":
			i++
			if i < len(args) {
				typeFilter = args[i]
			}
		case "--tag":
			i++
			if i < len(args) {
				tagFilter = args[i]
			}
		case "--status":
			i++
			if i < len(args) {
				statusFilter = args[i]
			}
		case "--confidence":
			i++
			if i < len(args) {
				confidenceFilter = args[i]
			}
		case "--origin":
			i++
			if i < len(args) {
				originFilter = args[i]
			}
		case "--zone":
			i++
			if i < len(args) {
				zoneFilter = args[i]
			}
		case "--past", "-p":
			i++
			if i < len(args) {
				pastFilter = args[i]
			}
		case "--tier":
			i++
			if i < len(args) {
				fmt.Sscanf(args[i], "%d", &tierFilter)
			}
		case "--old":
			oldFilter = true
		case "--gap":
			gapFilter = true
		case "--where":
			i++
			if i < len(args) {
				whereExpr = args[i]
			}
		case "--content-match":
			i++
			if i < len(args) {
				contentMatch = args[i]
			}
		case "--has-section":
			i++
			if i < len(args) {
				hasSection = args[i]
			}
		case "--fields", "-f":
			i++
			if i < len(args) {
				fieldsSpec = args[i]
			}
		case "--group-by", "-g":
			i++
			if i < len(args) {
				groupByField = args[i]
			}
		case "--sort", "-S":
			i++
			if i < len(args) {
				sortField = args[i]
			}
		case "--limit", "-l":
			i++
			if i < len(args) {
				fmt.Sscanf(args[i], "%d", &limit)
			}
		case "--count", "-c":
			countOnly = true
		default:
			if !strings.HasPrefix(args[i], "-") {
				positional = args[i]
			}
		}
	}

	// Build pipeline: cheap filters first, expensive last
	pipe := Query(items).
		Type(typeFilter).
		Status(statusFilter).
		Tag(tagFilter).
		Confidence(confidenceFilter).
		Origin(originFilter).
		Zone(zoneFilter).
		MinTier(tierFilter).
		Past(pastFilter).
		Where(func() []Predicate {
			if whereExpr == "" {
				return nil
			}
			return parseWhere(whereExpr)
		}()).
		HasSection(hasSection)

	// Conditional filters (boolean flags)
	if gapFilter {
		pipe.Gap()
	}
	if oldFilter {
		pipe.Stale()
	}

	// Positional: type name or text search
	if positional != "" {
		q := strings.ToLower(positional)
		if knownTypes[q] {
			pipe.Type(q)
		} else {
			pipe.TextSearch(q)
		}
	}

	// Content match (expensive — applied last, after all cheap filters)
	pipe.ContentMatch(contentMatch)

	// Sort + limit
	if sortField != "" {
		pipe.Sort(sortField)
	} else {
		pipe.SortDefault()
	}
	pipe.Limit(limit)

	items = pipe.Items()

	// Output
	if countOnly {
		if mode == ModeJSON {
			data, _ := json.Marshal(map[string]interface{}{"count": len(items)})
			fmt.Println(string(data))
		} else {
			fmt.Println(len(items))
		}
		return
	}

	if groupByField != "" {
		outputGroupBy(items, groupByField, mode)
		return
	}

	if fieldsSpec != "" {
		outputListProjected(items, parseFields(fieldsSpec), mode)
	} else {
		outputList(items, mode)
	}

	// Hints (only for default output, not projected/grouped, not quiet)
	if !quietMode && mode != ModeJSON && len(items) > 0 && fieldsSpec == "" && groupByField == "" {
		t0 := 0
		for _, a := range items {
			if a.Tier == 0 {
				t0++
			}
		}
		if t0 > 0 && t0*100/len(items) > 30 {
			fmt.Printf("hint: %d/%d at tier 0. try: ./ask lint --fix\n", t0, len(items))
		}
	}
}

// --- get ---

func cmdGet(args []string, mode OutputMode, idx *Index) error {
	if len(args) == 0 {
		return &UsageError{Msg: "usage: ask get <query>[,q2,...] [--meta] [--sec] [--diag] [--only f1,f2]"}
	}

	var meta, sec, diag bool
	var onlySpec string
	var queryParts []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--meta", "-m":
			meta = true
		case "--sec":
			sec = true
		case "--diag", "-d":
			diag = true
			meta = true
		case "--only":
			i++
			if i < len(args) {
				onlySpec = args[i]
			}
		default:
			if !strings.HasPrefix(args[i], "-") {
				queryParts = append(queryParts, args[i])
			}
		}
	}

	query := strings.Join(queryParts, " ")

	// Extract #section BEFORE multi-get check (id#block1,block2 is NOT multi-get)
	var section string
	if idx := strings.Index(query, "#"); idx >= 0 {
		section = query[idx+1:]
		query = query[:idx]
	}

	// Multi-get: comma-separated IDs (no spaces, no section)
	if section == "" && strings.Contains(query, ",") && !strings.Contains(query, " ") {
		ids := strings.Split(query, ",")
		var results []Artifact
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
			if a, ok := match.(Artifact); ok {
				results = append(results, a)
			} else if matches, ok := match.([]Artifact); ok && len(matches) > 0 {
				results = append(results, matches[0])
			}
		}
		if len(results) == 0 {
			return &NotFoundError{What: "no matching artifacts"}
		}
		outputMultiGet(results, onlySpec, mode, idx)
		return nil
	}

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

	// --only projection for single get
	if onlySpec != "" && !meta && !sec && section == "" {
		fields := parseFields(onlySpec)
		if mode == ModeJSON {
			data, _ := json.Marshal(projectArtifact(a, fields))
			fmt.Println(string(data))
		} else {
			for _, f := range fields {
				v := fieldValue(a, f)
				if mode == ModeWide {
					fmt.Printf("%s: %s\n", f, v)
				} else {
					fmt.Printf("%s\t%s\n", f, v)
				}
			}
		}
		return nil
	}

	if meta {
		outputMeta(a, mode, idx, diag)
		return nil
	}

	if sec {
		if mode == ModeJSON {
			data, _ := json.Marshal(map[string]interface{}{"id": a.ID, "sections": a.Sections})
			fmt.Println(string(data))
		} else {
			for _, s := range a.Sections {
				fmt.Println(s)
			}
		}
		return nil
	}

	content, err := os.ReadFile(filepath.Join(root, a.Path))
	if err != nil {
		return &FileError{Path: a.Path, Err: err}
	}

	if section != "" {
		_, bodyStart := parseFrontmatter(string(content))
		// Multi-block: id#block1,block2
		if strings.Contains(section, ",") {
			queries := strings.Split(section, ",")
			combined := getMultiBlockContent(string(content), bodyStart, queries)
			if mode == ModeJSON {
				data, _ := json.Marshal(map[string]interface{}{"id": a.ID, "sections": queries, "content": combined})
				fmt.Println(string(data))
			} else {
				fmt.Print(combined)
				if combined != "" && !strings.HasSuffix(combined, "\n") {
					fmt.Println()
				}
			}
			return nil
		}
		// Single block: try role-based resolution first, then legacy slug
		blockContent, found := getBlockContent(string(content), bodyStart, section)
		if !found {
			// Fallback to legacy getSectionContent
			blockContent = getSectionContent(string(content), section, bodyStart)
		}
		if mode == ModeJSON {
			data, _ := json.Marshal(map[string]interface{}{"id": a.ID, "section": section, "content": blockContent})
			fmt.Println(string(data))
		} else {
			fmt.Print(blockContent)
			if blockContent != "" && !strings.HasSuffix(blockContent, "\n") {
				fmt.Println()
			}
		}
		return nil
	}

	if mode == ModeJSON {
		_, bodyStart := parseFrontmatter(string(content))
		body := string(content)[bodyStart:]
		data, _ := json.Marshal(map[string]interface{}{
			"id":   a.ID,
			"path": a.Path,
			"meta": map[string]interface{}{
				"type": a.Type, "status": a.Status, "tags": a.Tags,
				"title": a.Title, "updated": a.Updated, "created": a.Created,
			},
			"content": body,
		})
		fmt.Println(string(data))
	} else {
		fmt.Print(string(content))
	}
	return nil
}

// --- sum ---

func cmdSum(args []string, mode OutputMode, idx *Index) {
	items := idx.Artifacts
	var typeOnly, tagOnly bool

	for _, arg := range args {
		switch arg {
		case "--type":
			typeOnly = true
		case "--tag":
			tagOnly = true
		}
	}

	typeCounts := map[string]int{}
	tagCounts := map[string]int{}
	statusCounts := map[string]int{}
	zoneCounts := map[string]int{}
	zoneFM := map[string]int{}

	for _, a := range items {
		typeCounts[a.Type]++
		statusCounts[a.Status]++
		for _, t := range a.Tags {
			tagCounts[t]++
		}
		parts := strings.Split(a.Path, "/")
		zone := parts[0] + "/"
		if len(parts) > 2 {
			zone = strings.Join(parts[:2], "/") + "/"
		}
		zoneCounts[zone]++
		if a.Tier > 0 {
			zoneFM[zone]++
		}
	}

	if mode == ModeJSON {
		result := map[string]interface{}{
			"total":     len(items),
			"by_zone":   zoneCounts,
			"by_type":   typeCounts,
			"by_status": statusCounts,
			"by_tag":    tagCounts,
			"coverage": map[string]interface{}{
				"with_fm": countTierAbove(items, 1),
				"total":   len(items),
				"pct":     pctTierAbove(items, 1),
			},
		}
		data, _ := json.Marshal(result)
		fmt.Println(string(data))
		return
	}

	if typeOnly {
		sorted := sortMapByValue(typeCounts)
		t := NewTable("type", "count")
		for _, kv := range sorted {
			t.Row(kv.Key, fmt.Sprintf("%d", kv.Value))
		}
		t.Render(mode)
		return
	}

	if tagOnly {
		sorted := sortMapByValue(tagCounts)
		t := NewTable("tag", "count")
		for _, kv := range sorted {
			t.Row(kv.Key, fmt.Sprintf("%d", kv.Value))
		}
		t.Render(mode)
		return
	}

	sumMeet := 0
	for _, a := range items {
		if a.Tier >= requiredTierFor(a) {
			sumMeet++
		}
	}
	sumCompliancePct := compliancePct(sumMeet, len(items))

	if mode == ModeWide {
		fmt.Printf("%-30s %5s  %4s\n", "ZONE", "FILES", "FM%")
		sorted := sortMapByKey(zoneCounts)
		for _, kv := range sorted {
			pct := compliancePct(zoneFM[kv.Key], kv.Value)
			marker := ""
			if pct == 0 && kv.Value > 3 {
				marker = "  !"
			}
			fmt.Printf("%-30s %5d  %3d%%%s\n", kv.Key, kv.Value, pct, marker)
		}
		withFM := countTierAbove(items, 1)
		pct := compliancePct(withFM, len(items))
		fmt.Printf("%-30s %5d  %3d%%\n", "TOTAL", len(items), pct)
		fmt.Printf("\nCOMPLIANCE: %d%%\n", sumCompliancePct)
		fmt.Println()
		fmt.Print("TYPES: ")
		for i, kv := range sortMapByValue(typeCounts) {
			if i > 0 {
				fmt.Print(" ")
			}
			fmt.Printf("%s(%d)", kv.Key, kv.Value)
		}
		fmt.Println()
		fmt.Print("TAGS:  ")
		tagsSorted := sortMapByValue(tagCounts)
		limit := 10
		if len(tagsSorted) < limit {
			limit = len(tagsSorted)
		}
		for i := 0; i < limit; i++ {
			if i > 0 {
				fmt.Print(" ")
			}
			fmt.Printf("%s(%d)", tagsSorted[i].Key, tagsSorted[i].Value)
		}
		fmt.Println()
		if sumCompliancePct < 100 {
			fmt.Printf("hint: compliance %d%%. try: ./ask lint --gap\n", sumCompliancePct)
		}
	} else {
		withFM := countTierAbove(items, 1)
		pct := pctTierAbove(items, 1)
		fmt.Printf("%d artifacts, %d with FM (%d%%), compliance:%d%%\n", len(items), withFM, pct, sumCompliancePct)
		types := sortMapByValue(typeCounts)
		limit := 5
		if len(types) < limit {
			limit = len(types)
		}
		parts := make([]string, limit)
		for i := 0; i < limit; i++ {
			parts[i] = fmt.Sprintf("%s:%d", types[i].Key, types[i].Value)
		}
		fmt.Printf("types: %s\n", strings.Join(parts, " "))
		tags := sortMapByValue(tagCounts)
		limit = 5
		if len(tags) < limit {
			limit = len(tags)
		}
		parts = make([]string, limit)
		for i := 0; i < limit; i++ {
			parts[i] = fmt.Sprintf("%s:%d", tags[i].Key, tags[i].Value)
		}
		fmt.Printf("tags: %s\n", strings.Join(parts, " "))
		if sumCompliancePct < 100 {
			fmt.Printf("hint: compliance %d%%. try: ./ask lint --gap\n", sumCompliancePct)
		}
	}
}

// --- map ---

func cmdMap(args []string, mode OutputMode, idx *Index) {
	var orphans bool
	var queryParts []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--orphans":
			orphans = true
		default:
			if !strings.HasPrefix(args[i], "-") {
				queryParts = append(queryParts, args[i])
			}
		}
	}

	items := idx.Artifacts

	if orphans {
		allPaths := map[string]bool{}
		hasEdge := map[string]bool{}
		for _, a := range items {
			allPaths[a.Path] = true
		}
		for _, a := range items {
			if len(a.EdgesOut) > 0 {
				hasEdge[a.Path] = true
				for _, e := range a.EdgesOut {
					if allPaths[e] {
						hasEdge[e] = true
					}
				}
			}
			if a.Source != "" {
				hasEdge[a.Path] = true
			}
		}
		for _, a := range items {
			for _, e := range a.EdgesOut {
				if allPaths[e] {
					hasEdge[e] = true
				}
			}
		}
		orphanList := filterArtifacts(items, func(a Artifact) bool { return !hasEdge[a.Path] })
		outputList(orphanList, mode)
		return
	}

	if len(queryParts) == 0 {
		fmt.Fprintln(os.Stderr, "usage: ask map <query> [--orphans]")
		os.Exit(1)
	}

	query := strings.Join(queryParts, " ")
	match := resolveArtifact(query, items)
	if match == nil {
		fmt.Fprintln(os.Stderr, "not found")
		os.Exit(1)
	}
	if matches, ok := match.([]Artifact); ok {
		outputList(matches, mode)
		os.Exit(1)
	}

	a := match.(Artifact)
	pathToID := map[string]string{}
	for _, item := range items {
		pathToID[item.Path] = item.ID
	}

	type Edge struct {
		Target string `json:"target"`
		Type   string `json:"type"`
	}
	var edgesOut, edgesIn []Edge

	for _, e := range a.EdgesOut {
		targetID := pathToID[e]
		if targetID == "" {
			targetID = e
		}
		edgesOut = append(edgesOut, Edge{targetID, "link"})
	}
	if a.Source != "" {
		edgesOut = append(edgesOut, Edge{a.Source, "source"})
	}

	for _, item := range items {
		if item.ID == a.ID {
			continue
		}
		for _, e := range item.EdgesOut {
			if e == a.Path {
				edgesIn = append(edgesIn, Edge{item.ID, "link"})
			}
		}
		if item.Source == a.Path {
			edgesIn = append(edgesIn, Edge{item.ID, "source"})
		}
	}

	if mode == ModeJSON {
		data, _ := json.Marshal(map[string]interface{}{"id": a.ID, "edges_out": edgesOut, "edges_in": edgesIn})
		fmt.Println(string(data))
	} else {
		fmt.Println(a.ID)
		for _, e := range edgesOut {
			if mode == ModeWide {
				fmt.Printf("  -> %s (%s)\n", e.Target, e.Type)
			} else {
				fmt.Printf("  -> %s\n", e.Target)
			}
		}
		for _, e := range edgesIn {
			if mode == ModeWide {
				fmt.Printf("  <- %s (%s)\n", e.Target, e.Type)
			} else {
				fmt.Printf("  <- %s\n", e.Target)
			}
		}
	}
}

// --- tag ---

func cmdTag(args []string, mode OutputMode, idx *Index) {
	items := idx.Artifacts

	var positional string
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			positional = arg
		}
	}

	if positional != "" {
		tag := strings.ToLower(positional)
		filtered := filterArtifacts(items, func(a Artifact) bool {
			for _, t := range a.Tags {
				if strings.ToLower(t) == tag {
					return true
				}
			}
			return false
		})
		outputList(filtered, mode)
		return
	}

	counter := map[string]int{}
	for _, a := range items {
		for _, t := range a.Tags {
			counter[t]++
		}
	}

	if mode == ModeJSON {
		data, _ := json.Marshal(counter)
		fmt.Println(string(data))
		return
	}

	sorted := sortMapByValue(counter)
	t := NewTable("tag", "count")
	for _, kv := range sorted {
		t.Row(kv.Key, fmt.Sprintf("%d", kv.Value))
	}
	t.Render(mode)
}

// --- scan ---

func cmdScan(args []string, mode OutputMode) error {
	idx := buildIndex()
	if err := saveIndex(idx); err != nil {
		return &FileError{Path: "index.json", Err: err}
	}
	meet := 0
	for _, a := range idx.Artifacts {
		if a.Tier >= requiredTierFor(a) {
			meet++
		}
	}
	pct := compliancePct(meet, len(idx.Artifacts))
	if mode == ModeJSON {
		data, _ := json.Marshal(map[string]interface{}{
			"total":          idx.Meta.Total,
			"git_only":       idx.Meta.GitOnly,
			"compliance_pct": pct,
		})
		fmt.Println(string(data))
	} else {
		label := "disk"
		if idx.Meta.GitOnly {
			label = "git-only"
		}
		fmt.Printf("%d artifacts indexed (%s), compliance:%d%%\n", idx.Meta.Total, label, pct)
		if pct < 100 {
			fmt.Printf("hint: %d gaps. try: ./ask lint --gap\n", len(idx.Artifacts)-meet)
		}
	}
	return nil
}
