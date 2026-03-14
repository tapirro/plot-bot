package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdout runs fn and returns whatever it writes to os.Stdout.
func captureStdout(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestNewTable(t *testing.T) {
	tbl := NewTable("id", "name")
	if len(tbl.columns) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(tbl.columns))
	}
	if tbl.columns[0].Name != "id" || tbl.columns[1].Name != "name" {
		t.Fatalf("unexpected column names: %v", tbl.columns)
	}
}

func TestColChain(t *testing.T) {
	tbl := &Table{}
	tbl.Col("a", 10).Col("b", 20)
	if len(tbl.columns) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(tbl.columns))
	}
	if tbl.columns[0].Width != 10 || tbl.columns[1].Width != 20 {
		t.Fatalf("unexpected widths: %d, %d", tbl.columns[0].Width, tbl.columns[1].Width)
	}
}

func TestRowAndLen(t *testing.T) {
	tbl := NewTable("a", "b")
	if tbl.Len() != 0 {
		t.Fatalf("expected 0 rows, got %d", tbl.Len())
	}
	tbl.Row("1", "2")
	tbl.Row("3", "4")
	if tbl.Len() != 2 {
		t.Fatalf("expected 2 rows, got %d", tbl.Len())
	}
}

func TestRenderText(t *testing.T) {
	tbl := NewTable("id", "path")
	tbl.Row("abc", "/foo/bar")
	tbl.Row("def", "/baz")

	out := captureStdout(func() { tbl.Render(ModeAgent) })

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), out)
	}
	if !strings.Contains(lines[0], "abc\t/foo/bar") {
		t.Errorf("expected tab-separated, got %q", lines[0])
	}
}

func TestRenderTextNoHeader(t *testing.T) {
	tbl := NewTable("id", "path")
	tbl.Row("x", "y")
	out := captureStdout(func() { tbl.Render(ModeAgent) })
	// Text mode must NOT contain header
	if strings.Contains(strings.ToUpper(out), "ID") && strings.Contains(out, "PATH") {
		// Only fail if it looks like a header line (uppercase at start)
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if len(lines) > 1 {
			t.Errorf("text mode should not have header, got %q", out)
		}
	}
}

func TestRenderWide(t *testing.T) {
	tbl := NewTable("type", "status")
	tbl.Row("pattern", "active")
	tbl.Row("bet", "experiment")

	out := captureStdout(func() { tbl.Render(ModeWide) })

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (header + 2 rows), got %d: %q", len(lines), out)
	}
	// Header should be uppercase
	if !strings.Contains(lines[0], "TYPE") || !strings.Contains(lines[0], "STATUS") {
		t.Errorf("expected uppercase header, got %q", lines[0])
	}
	// Data rows
	if !strings.Contains(lines[1], "pattern") {
		t.Errorf("expected 'pattern' in row 1, got %q", lines[1])
	}
}

func TestRenderWideAutoWidth(t *testing.T) {
	tbl := NewTable("mycol", "another")
	tbl.Row("short", "a")
	tbl.Row("a-much-longer-value", "b")

	out := captureStdout(func() { tbl.Render(ModeWide) })

	// The column should be wide enough for "a-much-longer-value"
	lines := strings.Split(strings.TrimSpace(out), "\n")
	// Row with longer value should not be truncated
	if !strings.Contains(lines[2], "a-much-longer-value") {
		t.Errorf("expected full value in wide mode, got %q", lines[2])
	}
}

func TestRenderJSON(t *testing.T) {
	tbl := NewTable("id", "name")
	tbl.Row("abc", "Alice")
	tbl.Row("def", "Bob")

	out := captureStdout(func() { tbl.Render(ModeJSON) })

	var result struct {
		Items []map[string]string `json:"items"`
		Total int                 `json:"total"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	if result.Total != 2 {
		t.Errorf("expected total=2, got %d", result.Total)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}
	if result.Items[0]["id"] != "abc" || result.Items[0]["name"] != "Alice" {
		t.Errorf("unexpected first item: %v", result.Items[0])
	}
}

func TestRenderWithTotalQuiet(t *testing.T) {
	oldQuiet := quietMode
	quietMode = true
	defer func() { quietMode = oldQuiet }()

	tbl := NewTable("a")
	tbl.Row("x")

	out := captureStdout(func() { tbl.RenderWithTotal(ModeAgent) })
	if strings.Contains(out, "items") {
		t.Errorf("quiet mode should suppress total, got %q", out)
	}
}

func TestRenderWithTotalShown(t *testing.T) {
	oldQuiet := quietMode
	quietMode = false
	defer func() { quietMode = oldQuiet }()

	tbl := NewTable("a")
	tbl.Row("x")
	tbl.Row("y")

	out := captureStdout(func() { tbl.RenderWithTotal(ModeAgent) })
	if !strings.Contains(out, "2 items") {
		t.Errorf("expected '2 items' footer, got %q", out)
	}
}

func TestEmptyTable(t *testing.T) {
	tbl := NewTable("a", "b")

	// Text: should be empty
	out := captureStdout(func() { tbl.Render(ModeAgent) })
	if strings.TrimSpace(out) != "" {
		t.Errorf("empty table text should be empty, got %q", out)
	}

	// Wide: should be empty (no header for empty table)
	out = captureStdout(func() { tbl.Render(ModeWide) })
	if strings.TrimSpace(out) != "" {
		t.Errorf("empty table wide should be empty, got %q", out)
	}

	// JSON: should have empty items
	out = captureStdout(func() { tbl.Render(ModeJSON) })
	var result struct {
		Items []map[string]string `json:"items"`
		Total int                 `json:"total"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON for empty table: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("expected total=0, got %d", result.Total)
	}
}

func TestKnownFieldWidths(t *testing.T) {
	// Table with known field names should use fieldWidth values
	tbl := NewTable("type", "status")
	widths := tbl.computeWidths()
	if widths[0] != 14 { // fieldWidth("type") == 14
		t.Errorf("expected type width 14, got %d", widths[0])
	}
	if widths[1] != 10 { // fieldWidth("status") == 10
		t.Errorf("expected status width 10, got %d", widths[1])
	}
}

func TestExplicitWidthOverride(t *testing.T) {
	tbl := &Table{}
	tbl.Col("foo", 25)
	tbl.Row("bar")
	widths := tbl.computeWidths()
	if widths[0] != 25 {
		t.Errorf("expected explicit width 25, got %d", widths[0])
	}
}
