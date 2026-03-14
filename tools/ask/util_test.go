package main

import (
	"errors"
	"fmt"
	"testing"
)

// --- Error types ---

func TestUsageError_Message(t *testing.T) {
	err := &UsageError{Msg: "missing argument"}
	if err.Error() != "missing argument" {
		t.Errorf("expected 'missing argument', got %q", err.Error())
	}
}

func TestNotFoundError_Message(t *testing.T) {
	err := &NotFoundError{What: "artifact bet-hive"}
	want := "not found: artifact bet-hive"
	if err.Error() != want {
		t.Errorf("expected %q, got %q", want, err.Error())
	}
}

func TestFileError_Message(t *testing.T) {
	inner := fmt.Errorf("permission denied")
	err := &FileError{Path: "/tmp/test.md", Err: inner}
	want := "cannot read /tmp/test.md: permission denied"
	if err.Error() != want {
		t.Errorf("expected %q, got %q", want, err.Error())
	}
}

func TestErrorTypes_AreDistinct(t *testing.T) {
	var err error

	err = &UsageError{Msg: "bad"}
	var ue *UsageError
	if !errors.As(err, &ue) {
		t.Error("UsageError should match errors.As")
	}

	err = &NotFoundError{What: "x"}
	var nfe *NotFoundError
	if !errors.As(err, &nfe) {
		t.Error("NotFoundError should match errors.As")
	}

	err = &FileError{Path: "p", Err: fmt.Errorf("e")}
	var fe *FileError
	if !errors.As(err, &fe) {
		t.Error("FileError should match errors.As")
	}

	// Cross-type should NOT match
	err = &UsageError{Msg: "bad"}
	if errors.As(err, &nfe) {
		t.Error("UsageError should not match NotFoundError")
	}
}

// --- filterArtifacts ---

func TestFilterArtifacts_MatchAll(t *testing.T) {
	items := []Artifact{
		{ID: "a", Type: "guide"},
		{ID: "b", Type: "pattern"},
		{ID: "c", Type: "guide"},
	}
	got := filterArtifacts(items, func(a Artifact) bool { return a.Type == "guide" })
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	if got[0].ID != "a" || got[1].ID != "c" {
		t.Errorf("wrong IDs: %s, %s", got[0].ID, got[1].ID)
	}
}

func TestFilterArtifacts_NoneMatch(t *testing.T) {
	items := []Artifact{{ID: "a", Type: "guide"}}
	got := filterArtifacts(items, func(a Artifact) bool { return a.Type == "bet" })
	if len(got) != 0 {
		t.Fatalf("expected 0, got %d", len(got))
	}
}

func TestFilterArtifacts_EmptyInput(t *testing.T) {
	got := filterArtifacts(nil, func(a Artifact) bool { return true })
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestFilterArtifacts_AllMatch(t *testing.T) {
	items := []Artifact{{ID: "x"}, {ID: "y"}}
	got := filterArtifacts(items, func(a Artifact) bool { return true })
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
}

// --- findByID ---

func TestFindByID_Found(t *testing.T) {
	idx := &Index{Artifacts: []Artifact{
		{ID: "bet-hive", Type: "bet"},
		{ID: "guide-concept", Type: "guide"},
	}}
	got := findByID(idx, "guide-concept")
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if got.Type != "guide" {
		t.Errorf("expected type guide, got %s", got.Type)
	}
}

func TestFindByID_NotFound(t *testing.T) {
	idx := &Index{Artifacts: []Artifact{{ID: "a"}}}
	got := findByID(idx, "nonexistent")
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestFindByID_EmptyIndex(t *testing.T) {
	idx := &Index{}
	got := findByID(idx, "anything")
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestFindByID_ReturnsPointerToOriginal(t *testing.T) {
	idx := &Index{Artifacts: []Artifact{{ID: "x", Title: "old"}}}
	got := findByID(idx, "x")
	got.Title = "new"
	if idx.Artifacts[0].Title != "new" {
		t.Error("expected pointer to original slice element")
	}
}

// --- compliancePct ---

func TestCompliancePct_Normal(t *testing.T) {
	got := compliancePct(3, 4)
	if got != 75 {
		t.Errorf("expected 75, got %d", got)
	}
}

func TestCompliancePct_ZeroTotal(t *testing.T) {
	got := compliancePct(0, 0)
	if got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestCompliancePct_Full(t *testing.T) {
	got := compliancePct(10, 10)
	if got != 100 {
		t.Errorf("expected 100, got %d", got)
	}
}

func TestCompliancePct_ZeroNumerator(t *testing.T) {
	got := compliancePct(0, 5)
	if got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestCompliancePct_IntegerDivision(t *testing.T) {
	// 1/3 = 33 (integer division)
	got := compliancePct(1, 3)
	if got != 33 {
		t.Errorf("expected 33, got %d", got)
	}
}

// --- isActionsHeading ---

func TestIsActionsHeading_English(t *testing.T) {
	if !isActionsHeading("## Actions") {
		t.Error("should match English")
	}
}

func TestIsActionsHeading_EnglishWithSuffix(t *testing.T) {
	if !isActionsHeading("## Actions & Decisions") {
		t.Error("should match prefix")
	}
}

func TestIsActionsHeading_Russian(t *testing.T) {
	if !isActionsHeading("## Действия") {
		t.Error("should match Russian")
	}
}

func TestIsActionsHeading_NonMatch(t *testing.T) {
	cases := []string{
		"## Summary",
		"# Actions",       // h1, not h2
		"### Actions",     // h3
		"## action items", // lowercase, different prefix
		"",
	}
	for _, c := range cases {
		if isActionsHeading(c) {
			t.Errorf("should not match: %q", c)
		}
	}
}

// --- sortMapByValue ---

func TestSortMapByValue_DescOrder(t *testing.T) {
	m := map[string]int{"a": 3, "b": 1, "c": 5}
	got := sortMapByValue(m)
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
	if got[0].Key != "c" || got[1].Key != "a" || got[2].Key != "b" {
		t.Errorf("wrong order: %v", got)
	}
}

func TestSortMapByValue_TieBreaker(t *testing.T) {
	m := map[string]int{"b": 1, "a": 1}
	got := sortMapByValue(m)
	// Equal values should sort alphabetically by key
	if got[0].Key != "a" || got[1].Key != "b" {
		t.Errorf("tie should sort by key: %v", got)
	}
}

func TestSortMapByValue_Empty(t *testing.T) {
	got := sortMapByValue(map[string]int{})
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

// --- countTierAbove / pctTierAbove ---

func TestCountTierAbove(t *testing.T) {
	items := []Artifact{{Tier: 0}, {Tier: 1}, {Tier: 2}, {Tier: 3}}
	if got := countTierAbove(items, 2); got != 2 {
		t.Errorf("expected 2, got %d", got)
	}
}

func TestPctTierAbove_Empty(t *testing.T) {
	if got := pctTierAbove(nil, 1); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestPctTierAbove_Normal(t *testing.T) {
	items := []Artifact{{Tier: 0}, {Tier: 1}, {Tier: 2}, {Tier: 3}}
	// 3 out of 4 have tier >= 1
	if got := pctTierAbove(items, 1); got != 75 {
		t.Errorf("expected 75, got %d", got)
	}
}
