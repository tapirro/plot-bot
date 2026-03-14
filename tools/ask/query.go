package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// --- WHERE predicate engine ---

type Predicate struct {
	Field string
	Op    string // =, !=, >, <, >=, <=, ~ (contains)
	Value string
}

// parseWhere splits "field op value AND field op value" into predicates.
func parseWhere(expr string) []Predicate {
	var preds []Predicate
	for _, part := range splitAND(expr) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if p := parsePredicate(part); p.Field != "" {
			preds = append(preds, p)
		}
	}
	return preds
}

func splitAND(s string) []string {
	var parts []string
	upper := strings.ToUpper(s)
	for {
		idx := strings.Index(upper, " AND ")
		if idx < 0 {
			parts = append(parts, s)
			break
		}
		parts = append(parts, s[:idx])
		s = s[idx+5:]
		upper = upper[idx+5:]
	}
	return parts
}

func parsePredicate(s string) Predicate {
	for _, op := range []string{">=", "<=", "!=", "~", "=", ">", "<"} {
		idx := strings.Index(s, op)
		if idx >= 0 {
			field := strings.TrimSpace(s[:idx])
			value := strings.TrimSpace(s[idx+len(op):])
			value = strings.Trim(value, "'\"")
			return Predicate{Field: field, Op: op, Value: value}
		}
	}
	return Predicate{}
}

// fieldValue extracts a string representation of any artifact field.
func fieldValue(a Artifact, field string) string {
	switch strings.ToLower(field) {
	case "id":
		return a.ID
	case "type":
		return a.Type
	case "status":
		return a.Status
	case "title":
		return a.Title
	case "path":
		return a.Path
	case "updated":
		return a.Updated
	case "created":
		return a.Created
	case "domain":
		return a.Domain
	case "source":
		return a.Source
	case "tier":
		return strconv.Itoa(a.Tier)
	case "tags":
		return strings.Join(a.Tags, ",")
	case "sections":
		return strings.Join(a.Sections, ",")
	case "edges_out":
		return strings.Join(a.EdgesOut, ",")
	case "confidence":
		return a.Confidence
	case "origin":
		return a.Origin
	case "basis":
		return a.Basis
	}
	return ""
}

func evalPredicate(a Artifact, p Predicate) bool {
	val := fieldValue(a, p.Field)
	switch p.Op {
	case "=":
		// Domain and origin fields: prefix match (e.g. "domain=hilart" matches "hilart/ops", "origin=harvest" matches "harvest/hilart-ops")
		if p.Field == "domain" || p.Field == "origin" {
			return strings.EqualFold(val, p.Value) ||
				strings.HasPrefix(strings.ToLower(val), strings.ToLower(p.Value)+"/")
		}
		return strings.EqualFold(val, p.Value)
	case "!=":
		return !strings.EqualFold(val, p.Value)
	case "~":
		return strings.Contains(strings.ToLower(val), strings.ToLower(p.Value))
	case ">":
		if fv, e1 := strconv.ParseFloat(val, 64); e1 == nil {
			if pv, e2 := strconv.ParseFloat(p.Value, 64); e2 == nil {
				return fv > pv
			}
		}
		return val > p.Value
	case "<":
		if fv, e1 := strconv.ParseFloat(val, 64); e1 == nil {
			if pv, e2 := strconv.ParseFloat(p.Value, 64); e2 == nil {
				return fv < pv
			}
		}
		return val < p.Value
	case ">=":
		if fv, e1 := strconv.ParseFloat(val, 64); e1 == nil {
			if pv, e2 := strconv.ParseFloat(p.Value, 64); e2 == nil {
				return fv >= pv
			}
		}
		return val >= p.Value
	case "<=":
		if fv, e1 := strconv.ParseFloat(val, 64); e1 == nil {
			if pv, e2 := strconv.ParseFloat(p.Value, 64); e2 == nil {
				return fv <= pv
			}
		}
		return val <= p.Value
	}
	return false
}

func filterWhere(items []Artifact, preds []Predicate) []Artifact {
	var result []Artifact
	for _, a := range items {
		match := true
		for _, p := range preds {
			if !evalPredicate(a, p) {
				match = false
				break
			}
		}
		if match {
			result = append(result, a)
		}
	}
	return result
}

// --- Content-aware filters ---

// filterContentMatch greps file content. Expensive — apply after cheaper filters.
func filterContentMatch(items []Artifact, pattern string) []Artifact {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return items
	}
	var result []Artifact
	for _, a := range items {
		data, err := os.ReadFile(filepath.Join(root, a.Path))
		if err != nil {
			continue
		}
		if re.Match(data) {
			result = append(result, a)
		}
	}
	return result
}

// filterHasSection matches artifacts that contain a section matching the slug.
func filterHasSection(items []Artifact, slug string) []Artifact {
	q := strings.ToLower(slug)
	var result []Artifact
	for _, a := range items {
		for _, s := range a.Sections {
			if strings.ToLower(s) == q || strings.Contains(strings.ToLower(s), q) {
				result = append(result, a)
				break
			}
		}
	}
	return result
}

// --- Field projection ---

func parseFields(spec string) []string {
	var fields []string
	for _, f := range strings.Split(spec, ",") {
		f = strings.TrimSpace(strings.ToLower(f))
		if f != "" {
			fields = append(fields, f)
		}
	}
	return fields
}

func projectArtifact(a Artifact, fields []string) map[string]interface{} {
	m := make(map[string]interface{})
	for _, f := range fields {
		switch f {
		case "tags":
			m["tags"] = a.Tags
		case "sections":
			m["sections"] = a.Sections
		case "edges_out":
			m["edges_out"] = a.EdgesOut
		case "tier":
			m["tier"] = a.Tier
		default:
			if v := fieldValue(a, f); v != "" {
				m[f] = v
			}
		}
	}
	return m
}
