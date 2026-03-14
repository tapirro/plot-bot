package main

import (
	"testing"
)

// --- parseFrontmatter ---

func TestParseFrontmatter_Normal(t *testing.T) {
	content := "---\ntype: bet\nstatus: active\ntags: [hilart, ops]\n---\n# Title\nBody"
	fm, bodyStart := parseFrontmatter(content)
	if fm == nil {
		t.Fatal("expected non-nil frontmatter")
	}
	if fmString(fm, "type") != "bet" {
		t.Errorf("type = %q", fmString(fm, "type"))
	}
	if fmString(fm, "status") != "active" {
		t.Errorf("status = %q", fmString(fm, "status"))
	}
	// bodyStart should point past the closing ---
	rest := content[bodyStart:]
	if rest != "# Title\nBody" {
		t.Errorf("body = %q", rest)
	}
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	content := "# Just a heading\nSome text"
	fm, bodyStart := parseFrontmatter(content)
	if fm != nil {
		t.Error("should be nil when no frontmatter")
	}
	if bodyStart != 0 {
		t.Errorf("bodyStart should be 0, got %d", bodyStart)
	}
}

func TestParseFrontmatter_MalformedNoClosing(t *testing.T) {
	content := "---\ntype: bet\nno closing marker"
	fm, bodyStart := parseFrontmatter(content)
	if fm != nil {
		t.Error("should be nil when no closing ---")
	}
	if bodyStart != 0 {
		t.Errorf("bodyStart should be 0, got %d", bodyStart)
	}
}

func TestParseFrontmatter_EmptyFrontmatter(t *testing.T) {
	content := "---\n---\n# Title"
	fm, bodyStart := parseFrontmatter(content)
	if fm == nil {
		t.Fatal("should return empty map, not nil")
	}
	if len(fm) != 0 {
		t.Errorf("expected empty map, got %v", fm)
	}
	if bodyStart <= 0 {
		t.Error("bodyStart should be positive")
	}
}

func TestParseFrontmatter_OnlyDelimiters(t *testing.T) {
	content := "---\n---"
	fm, _ := parseFrontmatter(content)
	if fm == nil {
		t.Fatal("should parse even minimal frontmatter")
	}
}

// --- parseSimpleYAML ---

func TestParseSimpleYAML_StringValues(t *testing.T) {
	text := "type: bet\nstatus: active\ntitle: My Title"
	m := parseSimpleYAML(text)
	if m["type"] != "bet" {
		t.Errorf("type = %v", m["type"])
	}
	if m["status"] != "active" {
		t.Errorf("status = %v", m["status"])
	}
	if m["title"] != "My Title" {
		t.Errorf("title = %v", m["title"])
	}
}

func TestParseSimpleYAML_InlineList(t *testing.T) {
	text := "tags: [hilart, ops, dwh]"
	m := parseSimpleYAML(text)
	tags, ok := m["tags"].([]string)
	if !ok {
		t.Fatalf("tags should be []string, got %T", m["tags"])
	}
	if len(tags) != 3 {
		t.Fatalf("expected 3 tags, got %d", len(tags))
	}
	if tags[0] != "hilart" || tags[1] != "ops" || tags[2] != "dwh" {
		t.Errorf("wrong tags: %v", tags)
	}
}

func TestParseSimpleYAML_QuotedInlineList(t *testing.T) {
	text := `tags: ["hilart", 'ops']`
	m := parseSimpleYAML(text)
	tags, ok := m["tags"].([]string)
	if !ok {
		t.Fatalf("tags should be []string, got %T", m["tags"])
	}
	if len(tags) != 2 || tags[0] != "hilart" || tags[1] != "ops" {
		t.Errorf("quoted values should be stripped: %v", tags)
	}
}

func TestParseSimpleYAML_Comments(t *testing.T) {
	text := "# this is a comment\ntype: bet"
	m := parseSimpleYAML(text)
	if _, ok := m["# this is a comment"]; ok {
		t.Error("comment lines should be skipped")
	}
	if m["type"] != "bet" {
		t.Errorf("type = %v", m["type"])
	}
}

func TestParseSimpleYAML_InlineComment(t *testing.T) {
	text := "status: active # current state"
	m := parseSimpleYAML(text)
	if m["status"] != "active" {
		t.Errorf("inline comment should be stripped: got %q", m["status"])
	}
}

func TestParseSimpleYAML_IndentedLinesSkipped(t *testing.T) {
	text := "type: bet\n  nested: value\n\tindented: tab"
	m := parseSimpleYAML(text)
	if _, ok := m["nested"]; ok {
		t.Error("indented (space) lines should be skipped")
	}
	if _, ok := m["indented"]; ok {
		t.Error("indented (tab) lines should be skipped")
	}
	if m["type"] != "bet" {
		t.Errorf("type = %v", m["type"])
	}
}

func TestParseSimpleYAML_QuotedStringValue(t *testing.T) {
	text := `title: "My Title"`
	m := parseSimpleYAML(text)
	if m["title"] != "My Title" {
		t.Errorf("quoted value should be stripped: got %q", m["title"])
	}
}

func TestParseSimpleYAML_EmptyValue(t *testing.T) {
	text := "domain:"
	m := parseSimpleYAML(text)
	if m["domain"] != "" {
		t.Errorf("empty value should be empty string, got %q", m["domain"])
	}
}

func TestParseSimpleYAML_EmptyInput(t *testing.T) {
	m := parseSimpleYAML("")
	if len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
}

func TestParseSimpleYAML_EmptyList(t *testing.T) {
	text := "tags: []"
	m := parseSimpleYAML(text)
	tags, ok := m["tags"].([]string)
	if !ok {
		t.Fatalf("expected []string, got %T", m["tags"])
	}
	if len(tags) != 0 {
		t.Errorf("expected empty list, got %v", tags)
	}
}

func TestParseSimpleYAML_ColonInValue(t *testing.T) {
	text := "title: Part A: The Beginning"
	m := parseSimpleYAML(text)
	if m["title"] != "Part A: The Beginning" {
		t.Errorf("colons in value should be preserved: got %q", m["title"])
	}
}

// --- fmString ---

func TestFmString_Present(t *testing.T) {
	fm := map[string]interface{}{"type": "bet"}
	if fmString(fm, "type") != "bet" {
		t.Error("should return string value")
	}
}

func TestFmString_Absent(t *testing.T) {
	fm := map[string]interface{}{"type": "bet"}
	if fmString(fm, "status") != "" {
		t.Error("absent key should return empty")
	}
}

func TestFmString_NilMap(t *testing.T) {
	if fmString(nil, "type") != "" {
		t.Error("nil map should return empty")
	}
}

func TestFmString_NonStringValue(t *testing.T) {
	fm := map[string]interface{}{"tags": []string{"a", "b"}}
	if fmString(fm, "tags") != "" {
		t.Error("non-string value should return empty")
	}
}

// --- fmStringList ---

func TestFmStringList_ListValue(t *testing.T) {
	fm := map[string]interface{}{"tags": []string{"a", "b"}}
	got := fmStringList(fm, "tags")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("should return list: got %v", got)
	}
}

func TestFmStringList_CommaString(t *testing.T) {
	fm := map[string]interface{}{"tags": "a, b, c"}
	got := fmStringList(fm, "tags")
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d: %v", len(got), got)
	}
	if got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("should split and trim: %v", got)
	}
}

func TestFmStringList_Absent(t *testing.T) {
	fm := map[string]interface{}{"type": "bet"}
	got := fmStringList(fm, "tags")
	if got != nil {
		t.Errorf("absent key should return nil, got %v", got)
	}
}

func TestFmStringList_NilMap(t *testing.T) {
	got := fmStringList(nil, "tags")
	if got != nil {
		t.Errorf("nil map should return nil, got %v", got)
	}
}

func TestFmStringList_EmptyString(t *testing.T) {
	fm := map[string]interface{}{"tags": ""}
	got := fmStringList(fm, "tags")
	if got != nil {
		t.Errorf("empty string should return nil, got %v", got)
	}
}

func TestFmStringList_SingleItem(t *testing.T) {
	fm := map[string]interface{}{"tags": "solo"}
	got := fmStringList(fm, "tags")
	if len(got) != 1 || got[0] != "solo" {
		t.Errorf("single item: %v", got)
	}
}

// --- getTitle ---

func TestGetTitle_Normal(t *testing.T) {
	content := "---\ntype: bet\n---\n# My Title\nBody text"
	_, bodyStart := parseFrontmatter(content)
	got := getTitle(content, bodyStart)
	if got != "My Title" {
		t.Errorf("expected 'My Title', got %q", got)
	}
}

func TestGetTitle_SkipsH2(t *testing.T) {
	content := "## Section\n# Real Title\nBody"
	got := getTitle(content, 0)
	if got != "Real Title" {
		t.Errorf("should skip ## and find #: got %q", got)
	}
}

func TestGetTitle_NoHeading(t *testing.T) {
	content := "Just body text\nNo heading here"
	got := getTitle(content, 0)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// --- slugify ---

func TestSlugify(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"Action Items", "action-items"},
		{"Принятые решения", "принятые-решения"},
		{"Hello_World", "hello-world"},
		{"Multiple   Spaces", "multiple-spaces"},
		{"Special!@#Chars", "special-chars"},
		{"", ""},
	}
	for _, tc := range cases {
		got := slugify(tc.input)
		if got != tc.want {
			t.Errorf("slugify(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// --- getSections ---

func TestGetSections_Normal(t *testing.T) {
	content := "# Title\n## Summary\nText\n## Solution\nMore text\n### Subsection"
	got := getSections(content, 0)
	if len(got) != 2 {
		t.Fatalf("expected 2 sections, got %d: %v", len(got), got)
	}
	if got[0] != "summary" || got[1] != "solution" {
		t.Errorf("wrong sections: %v", got)
	}
}

func TestGetSections_NoSections(t *testing.T) {
	content := "# Title\nJust body, no H2"
	got := getSections(content, 0)
	if len(got) != 0 {
		t.Errorf("expected 0, got %v", got)
	}
}
