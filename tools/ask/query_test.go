package main

import (
	"testing"
)

// --- fieldValue ---

func TestFieldValue_AllFields(t *testing.T) {
	a := Artifact{
		ID: "test-id", Type: "pattern", Status: "active",
		Title: "Test Title", Path: "knowledge/test.md",
		Updated: "2026-01-01", Created: "2025-12-01",
		Domain: "hilart/ops", Source: "fireflies",
		Tier: 2, Tags: []string{"tag1", "tag2"},
		Sections:   []string{"summary", "solution"},
		EdgesOut:   []string{"ref-a", "ref-b"},
		Confidence: "validated", Origin: "harvest/hilart-ops",
		Basis: "3 incidents",
	}
	cases := []struct {
		field string
		want  string
	}{
		{"id", "test-id"},
		{"type", "pattern"},
		{"status", "active"},
		{"title", "Test Title"},
		{"path", "knowledge/test.md"},
		{"updated", "2026-01-01"},
		{"created", "2025-12-01"},
		{"domain", "hilart/ops"},
		{"source", "fireflies"},
		{"tier", "2"},
		{"tags", "tag1,tag2"},
		{"sections", "summary,solution"},
		{"edges_out", "ref-a,ref-b"},
		{"confidence", "validated"},
		{"origin", "harvest/hilart-ops"},
		{"basis", "3 incidents"},
	}
	for _, tc := range cases {
		got := fieldValue(a, tc.field)
		if got != tc.want {
			t.Errorf("fieldValue(%q) = %q, want %q", tc.field, got, tc.want)
		}
	}
}

func TestFieldValue_CaseInsensitive(t *testing.T) {
	a := Artifact{ID: "x"}
	if fieldValue(a, "ID") != "x" {
		t.Error("should be case-insensitive")
	}
	if fieldValue(a, "Id") != "x" {
		t.Error("should be case-insensitive")
	}
}

func TestFieldValue_UnknownField(t *testing.T) {
	a := Artifact{ID: "x"}
	if fieldValue(a, "nonexistent") != "" {
		t.Error("unknown field should return empty")
	}
}

func TestFieldValue_EmptyArtifact(t *testing.T) {
	a := Artifact{}
	if fieldValue(a, "id") != "" {
		t.Error("empty artifact id should be empty")
	}
	if fieldValue(a, "tier") != "0" {
		t.Error("empty artifact tier should be '0'")
	}
}

// --- projectArtifact ---

func TestProjectArtifact_ScalarFields(t *testing.T) {
	a := Artifact{ID: "x", Type: "bet", Domain: "hilart"}
	m := projectArtifact(a, []string{"id", "type", "domain"})
	if m["id"] != "x" {
		t.Errorf("id = %v", m["id"])
	}
	if m["type"] != "bet" {
		t.Errorf("type = %v", m["type"])
	}
	// This was a historical bug — domain must be included
	if m["domain"] != "hilart" {
		t.Errorf("domain missing or wrong: %v", m["domain"])
	}
}

func TestProjectArtifact_ArrayFields(t *testing.T) {
	a := Artifact{
		Tags:     []string{"a", "b"},
		Sections: []string{"summary"},
		EdgesOut: []string{"ref-1"},
	}
	m := projectArtifact(a, []string{"tags", "sections", "edges_out"})

	tags, ok := m["tags"].([]string)
	if !ok {
		t.Fatal("tags should be []string")
	}
	if len(tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(tags))
	}

	sections, ok := m["sections"].([]string)
	if !ok {
		t.Fatal("sections should be []string")
	}
	if len(sections) != 1 {
		t.Errorf("expected 1 section, got %d", len(sections))
	}

	edges, ok := m["edges_out"].([]string)
	if !ok {
		t.Fatal("edges_out should be []string")
	}
	if len(edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(edges))
	}
}

func TestProjectArtifact_TierIsInt(t *testing.T) {
	a := Artifact{Tier: 3}
	m := projectArtifact(a, []string{"tier"})
	tier, ok := m["tier"].(int)
	if !ok {
		t.Fatal("tier should be int")
	}
	if tier != 3 {
		t.Errorf("expected 3, got %d", tier)
	}
}

func TestProjectArtifact_EmptyFieldOmitted(t *testing.T) {
	a := Artifact{ID: "x"}
	m := projectArtifact(a, []string{"id", "domain"})
	if _, ok := m["domain"]; ok {
		t.Error("empty domain should be omitted from projection")
	}
	if m["id"] != "x" {
		t.Error("non-empty id should be present")
	}
}

func TestProjectArtifact_UnknownFieldIgnored(t *testing.T) {
	a := Artifact{ID: "x"}
	m := projectArtifact(a, []string{"id", "foobar"})
	if _, ok := m["foobar"]; ok {
		t.Error("unknown field should not appear")
	}
}

func TestProjectArtifact_NilTagsStillReturned(t *testing.T) {
	a := Artifact{Tags: nil}
	m := projectArtifact(a, []string{"tags"})
	// tags field should be present (even if nil) because it's special-cased
	if _, ok := m["tags"]; !ok {
		t.Error("tags field should be present even when nil")
	}
}

// --- parseWhere ---

func TestParseWhere_Single(t *testing.T) {
	preds := parseWhere("type=bet")
	if len(preds) != 1 {
		t.Fatalf("expected 1, got %d", len(preds))
	}
	if preds[0].Field != "type" || preds[0].Op != "=" || preds[0].Value != "bet" {
		t.Errorf("wrong predicate: %+v", preds[0])
	}
}

func TestParseWhere_AND(t *testing.T) {
	preds := parseWhere("type=bet AND status=active")
	if len(preds) != 2 {
		t.Fatalf("expected 2, got %d", len(preds))
	}
	if preds[0].Field != "type" || preds[0].Value != "bet" {
		t.Errorf("first pred wrong: %+v", preds[0])
	}
	if preds[1].Field != "status" || preds[1].Value != "active" {
		t.Errorf("second pred wrong: %+v", preds[1])
	}
}

func TestParseWhere_CaseInsensitiveAND(t *testing.T) {
	preds := parseWhere("type=bet and status=active")
	if len(preds) != 2 {
		t.Fatalf("expected 2 (lowercase 'and'), got %d", len(preds))
	}
}

func TestParseWhere_Empty(t *testing.T) {
	preds := parseWhere("")
	if len(preds) != 0 {
		t.Fatalf("expected 0, got %d", len(preds))
	}
}

func TestParseWhere_QuotedValue(t *testing.T) {
	preds := parseWhere("title~'my title'")
	if len(preds) != 1 {
		t.Fatalf("expected 1, got %d", len(preds))
	}
	if preds[0].Value != "my title" {
		t.Errorf("quotes should be stripped: got %q", preds[0].Value)
	}
}

func TestParseWhere_AllOps(t *testing.T) {
	ops := []string{"=", "!=", "~", ">", "<", ">=", "<="}
	for _, op := range ops {
		preds := parseWhere("tier" + op + "2")
		if len(preds) != 1 {
			t.Fatalf("op %s: expected 1 pred, got %d", op, len(preds))
		}
		if preds[0].Op != op {
			t.Errorf("expected op %q, got %q", op, preds[0].Op)
		}
	}
}

// --- evalPredicate ---

func TestEvalPredicate_Equals(t *testing.T) {
	a := Artifact{Type: "bet"}
	if !evalPredicate(a, Predicate{"type", "=", "bet"}) {
		t.Error("should match")
	}
	if evalPredicate(a, Predicate{"type", "=", "guide"}) {
		t.Error("should not match")
	}
}

func TestEvalPredicate_EqualsCaseInsensitive(t *testing.T) {
	a := Artifact{Type: "bet"}
	if !evalPredicate(a, Predicate{"type", "=", "BET"}) {
		t.Error("= should be case-insensitive")
	}
}

func TestEvalPredicate_NotEquals(t *testing.T) {
	a := Artifact{Type: "bet"}
	if !evalPredicate(a, Predicate{"type", "!=", "guide"}) {
		t.Error("should match != guide")
	}
	if evalPredicate(a, Predicate{"type", "!=", "bet"}) {
		t.Error("should not match != bet")
	}
}

func TestEvalPredicate_Contains(t *testing.T) {
	a := Artifact{Title: "Hive Master Plan"}
	if !evalPredicate(a, Predicate{"title", "~", "master"}) {
		t.Error("should contain 'master' (case-insensitive)")
	}
	if evalPredicate(a, Predicate{"title", "~", "xyz"}) {
		t.Error("should not contain 'xyz'")
	}
}

func TestEvalPredicate_NumericGreater(t *testing.T) {
	a := Artifact{Tier: 3}
	if !evalPredicate(a, Predicate{"tier", ">", "2"}) {
		t.Error("3 > 2")
	}
	if evalPredicate(a, Predicate{"tier", ">", "3"}) {
		t.Error("3 not > 3")
	}
}

func TestEvalPredicate_NumericLess(t *testing.T) {
	a := Artifact{Tier: 1}
	if !evalPredicate(a, Predicate{"tier", "<", "2"}) {
		t.Error("1 < 2")
	}
}

func TestEvalPredicate_NumericGTE(t *testing.T) {
	a := Artifact{Tier: 2}
	if !evalPredicate(a, Predicate{"tier", ">=", "2"}) {
		t.Error("2 >= 2")
	}
	if !evalPredicate(a, Predicate{"tier", ">=", "1"}) {
		t.Error("2 >= 1")
	}
	if evalPredicate(a, Predicate{"tier", ">=", "3"}) {
		t.Error("2 not >= 3")
	}
}

func TestEvalPredicate_NumericLTE(t *testing.T) {
	a := Artifact{Tier: 2}
	if !evalPredicate(a, Predicate{"tier", "<=", "2"}) {
		t.Error("2 <= 2")
	}
	if evalPredicate(a, Predicate{"tier", "<=", "1"}) {
		t.Error("2 not <= 1")
	}
}

func TestEvalPredicate_DomainPrefixMatch(t *testing.T) {
	a := Artifact{Domain: "hilart/ops"}
	// Exact match
	if !evalPredicate(a, Predicate{"domain", "=", "hilart/ops"}) {
		t.Error("exact domain should match")
	}
	// Prefix match: "hilart" should match "hilart/ops"
	if !evalPredicate(a, Predicate{"domain", "=", "hilart"}) {
		t.Error("domain prefix should match")
	}
	// Non-matching prefix
	if evalPredicate(a, Predicate{"domain", "=", "voic"}) {
		t.Error("wrong prefix should not match")
	}
}

func TestEvalPredicate_OriginPrefixMatch(t *testing.T) {
	a := Artifact{Origin: "harvest/hilart-ops"}
	if !evalPredicate(a, Predicate{"origin", "=", "harvest"}) {
		t.Error("origin prefix should match")
	}
	if !evalPredicate(a, Predicate{"origin", "=", "harvest/hilart-ops"}) {
		t.Error("exact origin should match")
	}
	if evalPredicate(a, Predicate{"origin", "=", "manual"}) {
		t.Error("wrong origin should not match")
	}
}

func TestEvalPredicate_StringComparison(t *testing.T) {
	a := Artifact{Updated: "2026-03-01"}
	if !evalPredicate(a, Predicate{"updated", ">", "2026-02-28"}) {
		t.Error("date string comparison should work")
	}
	if evalPredicate(a, Predicate{"updated", ">", "2026-03-02"}) {
		t.Error("should not be greater")
	}
}

// --- filterWhere ---

func TestFilterWhere_Combined(t *testing.T) {
	items := []Artifact{
		{ID: "a", Type: "bet", Status: "active"},
		{ID: "b", Type: "bet", Status: "proposed"},
		{ID: "c", Type: "guide", Status: "active"},
	}
	preds := []Predicate{
		{Field: "type", Op: "=", Value: "bet"},
		{Field: "status", Op: "=", Value: "active"},
	}
	got := filterWhere(items, preds)
	if len(got) != 1 || got[0].ID != "a" {
		t.Errorf("expected [a], got %v", got)
	}
}

func TestFilterWhere_NoPreds(t *testing.T) {
	items := []Artifact{{ID: "a"}, {ID: "b"}}
	got := filterWhere(items, nil)
	if len(got) != 2 {
		t.Errorf("no predicates should return all, got %d", len(got))
	}
}

func TestFilterWhere_EmptyItems(t *testing.T) {
	preds := []Predicate{{Field: "type", Op: "=", Value: "bet"}}
	got := filterWhere(nil, preds)
	if len(got) != 0 {
		t.Errorf("empty items should return empty, got %d", len(got))
	}
}

// --- filterHasSection ---

func TestFilterHasSection_ExactMatch(t *testing.T) {
	items := []Artifact{
		{ID: "a", Sections: []string{"summary", "solution"}},
		{ID: "b", Sections: []string{"problem"}},
	}
	got := filterHasSection(items, "solution")
	if len(got) != 1 || got[0].ID != "a" {
		t.Errorf("expected [a], got %v", got)
	}
}

func TestFilterHasSection_PartialMatch(t *testing.T) {
	items := []Artifact{
		{ID: "a", Sections: []string{"action-items"}},
	}
	got := filterHasSection(items, "action")
	if len(got) != 1 {
		t.Error("partial match should work via contains")
	}
}

func TestFilterHasSection_NoMatch(t *testing.T) {
	items := []Artifact{
		{ID: "a", Sections: []string{"summary"}},
	}
	got := filterHasSection(items, "nonexistent")
	if len(got) != 0 {
		t.Error("should find nothing")
	}
}

// --- parseFields ---

func TestParseFields_Simple(t *testing.T) {
	got := parseFields("id,type,status")
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
	if got[0] != "id" || got[1] != "type" || got[2] != "status" {
		t.Errorf("wrong fields: %v", got)
	}
}

func TestParseFields_WithSpaces(t *testing.T) {
	got := parseFields(" id , type , status ")
	if len(got) != 3 || got[0] != "id" {
		t.Errorf("should trim: %v", got)
	}
}

func TestParseFields_Lowercase(t *testing.T) {
	got := parseFields("ID,Type")
	if got[0] != "id" || got[1] != "type" {
		t.Errorf("should lowercase: %v", got)
	}
}

func TestParseFields_Empty(t *testing.T) {
	got := parseFields("")
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}
