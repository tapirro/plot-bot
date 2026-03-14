package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// --- Output modes ---

type OutputMode int

const (
	ModeAgent OutputMode = iota
	ModeJSON
	ModeWide
)

// --- List rendering ---

func outputList(items []Artifact, mode OutputMode) {
	if mode == ModeJSON {
		// Keep full struct for JSON — Table can't handle omitempty/int fields
		type listItem struct {
			ID         string   `json:"id"`
			Type       string   `json:"type"`
			Status     string   `json:"status"`
			Domain     string   `json:"domain,omitempty"`
			Tags       []string `json:"tags"`
			Title      string   `json:"title"`
			Path       string   `json:"path"`
			Updated    string   `json:"updated"`
			Created    string   `json:"created"`
			Tier       int      `json:"tier"`
			Confidence string   `json:"confidence,omitempty"`
			Origin     string   `json:"origin,omitempty"`
			Basis      string   `json:"basis,omitempty"`
		}
		var out []listItem
		for _, a := range items {
			out = append(out, listItem{a.ID, a.Type, a.Status, a.Domain, a.Tags, a.Title, a.Path, a.Updated, a.Created, a.Tier, a.Confidence, a.Origin, a.Basis})
		}
		result := map[string]interface{}{"items": out, "total": len(items)}
		jsonPrintCompact(result)
		return
	}

	var t *Table
	if mode == ModeWide {
		t = NewTable("type", "status", "updated", "tags", "title")
		for _, a := range items {
			tags := strings.Join(a.Tags, ",")
			if len(tags) > 15 {
				tags = tags[:15]
			}
			upd := ""
			if len(a.Updated) >= 10 {
				upd = a.Updated[5:10]
			}
			t.Row(a.Type, a.Status, upd, tags, a.Title)
		}
	} else {
		t = NewTable("id", "path")
		for _, a := range items {
			t.Row(a.ID, a.Path)
		}
	}
	t.RenderWithTotal(mode)
}

// --- Meta rendering ---

func outputMeta(a Artifact, mode OutputMode, idx *Index, diag bool) {
	if mode == ModeJSON {
		result := map[string]interface{}{
			"id": a.ID, "type": a.Type, "status": a.Status,
			"domain": a.Domain,
			"tags": a.Tags, "title": a.Title, "path": a.Path,
			"updated": a.Updated, "created": a.Created,
			"sections": a.Sections, "edges_out": a.EdgesOut,
			"source": a.Source, "tier": a.Tier,
			"confidence": a.Confidence, "origin": a.Origin, "basis": a.Basis,
		}
		if diag {
			req, missing := tierDiag(a)
			result["required_tier"] = req
			result["missing"] = missing
		}
		jsonPrintCompact(result)
		return
	}

	var edgesIn []string
	for _, item := range idx.Artifacts {
		if item.ID == a.ID {
			continue
		}
		for _, e := range item.EdgesOut {
			if e == a.Path {
				edgesIn = append(edgesIn, item.ID)
			}
		}
	}

	req, missing := tierDiag(a)

	if mode == ModeWide {
		fmt.Printf("id: %s\n", a.ID)
		fmt.Printf("type: %s\n", a.Type)
		fmt.Printf("status: %s\n", a.Status)
		fmt.Printf("tags: %s\n", strings.Join(a.Tags, ", "))
		fmt.Printf("title: %s\n", a.Title)
		fmt.Printf("path: %s\n", a.Path)
		if len(a.Sections) > 0 {
			fmt.Printf("sections: %s\n", strings.Join(a.Sections, ", "))
		}
		if len(a.EdgesOut) > 0 {
			fmt.Printf("edges_out: %s\n", strings.Join(a.EdgesOut, ", "))
		}
		if len(edgesIn) > 0 {
			fmt.Printf("edges_in: %s\n", strings.Join(edgesIn, ", "))
		}
		if a.Source != "" {
			fmt.Printf("source: %s\n", a.Source)
		}
		if a.Confidence != "" {
			fmt.Printf("confidence: %s\n", a.Confidence)
		}
		if a.Origin != "" {
			fmt.Printf("origin: %s\n", a.Origin)
		}
		if a.Basis != "" {
			fmt.Printf("basis: %s\n", a.Basis)
		}
		fmt.Printf("updated: %s\n", a.Updated)
		fmt.Printf("created: %s\n", a.Created)
		if len(missing) > 0 {
			fmt.Printf("tier: %d (need %d, missing: %s)\n", a.Tier, req, strings.Join(missing, ", "))
		} else {
			fmt.Printf("tier: %d\n", a.Tier)
		}
	} else {
		fmt.Printf("%s\t%s\n", a.ID, a.Path)
		if len(missing) > 0 {
			fmt.Printf("type:%s status:%s tier:%d (need %d, missing: %s)\n", a.Type, a.Status, a.Tier, req, strings.Join(missing, ", "))
		} else {
			fmt.Printf("type:%s status:%s tier:%d\n", a.Type, a.Status, a.Tier)
		}
		if len(a.Tags) > 0 {
			fmt.Printf("tags: %s\n", strings.Join(a.Tags, ", "))
		}
		if len(missing) > 0 {
			fmt.Printf("hint: add %s to frontmatter to reach tier %d\n", strings.Join(missing, ", "), req)
		}
	}
}

// --- Projected list rendering ---

func outputListProjected(items []Artifact, fields []string, mode OutputMode) {
	if mode == ModeJSON {
		var out []map[string]interface{}
		for _, a := range items {
			out = append(out, projectArtifact(a, fields))
		}
		jsonPrintCompact(map[string]interface{}{"items": out, "total": len(items)})
		return
	}

	t := NewTable(fields...)
	for _, a := range items {
		vals := make([]string, len(fields))
		for i, f := range fields {
			vals[i] = fieldValue(a, f)
		}
		t.Row(vals...)
	}
	t.RenderWithTotal(mode)
}

func fieldWidth(f string) int {
	switch f {
	case "id":
		return 30
	case "title":
		return 40
	case "path":
		return 50
	case "type":
		return 14
	case "status":
		return 10
	case "tags":
		return 20
	case "updated", "created":
		return 12
	case "tier":
		return 5
	case "confidence":
		return 12
	case "origin":
		return 25
	case "basis":
		return 35
	default:
		return 16
	}
}

// --- Group-by rendering ---

func outputGroupBy(items []Artifact, field string, mode OutputMode) {
	counts := map[string]int{}
	for _, a := range items {
		key := fieldValue(a, field)
		if key == "" {
			key = "(empty)"
		}
		counts[key]++
	}

	if mode == ModeJSON {
		jsonPrintCompact(map[string]interface{}{"field": field, "groups": counts, "total": len(items)})
		return
	}

	sorted := sortMapByValue(counts)
	t := NewTable(field, "count")
	for _, kv := range sorted {
		t.Row(kv.Key, fmt.Sprintf("%d", kv.Value))
	}
	t.Render(mode)
	fmt.Printf("%d items in %d groups\n", len(items), len(counts))
}

// --- Multi-get rendering ---

func outputMultiGet(items []Artifact, onlySpec string, mode OutputMode, idx *Index) {
	if onlySpec != "" {
		fields := parseFields(onlySpec)
		outputListProjected(items, fields, mode)
		return
	}

	if mode == ModeJSON {
		var results []map[string]interface{}
		for _, a := range items {
			content, err := os.ReadFile(filepath.Join(root, a.Path))
			body := ""
			if err == nil {
				_, bs := parseFrontmatter(string(content))
				body = string(content)[bs:]
			}
			results = append(results, map[string]interface{}{
				"id": a.ID, "path": a.Path,
				"meta": map[string]interface{}{
					"type": a.Type, "status": a.Status, "tags": a.Tags,
					"title": a.Title, "updated": a.Updated, "created": a.Created,
				},
				"content": body,
			})
		}
		jsonPrintCompact(map[string]interface{}{"items": results, "total": len(results)})
	} else {
		for i, a := range items {
			if i > 0 {
				fmt.Println("---")
			}
			content, err := os.ReadFile(filepath.Join(root, a.Path))
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: cannot read %s: %v\n", a.Path, err)
				continue
			}
			fmt.Print(string(content))
		}
	}
}
