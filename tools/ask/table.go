package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Table renders tabular data in text (tab-separated), wide (aligned columns), or JSON mode.
type Table struct {
	columns []tableCol
	rows    [][]string
}

type tableCol struct {
	Name  string
	Width int // 0 = auto-compute from data
}

// NewTable creates a table with named columns and auto-computed widths.
func NewTable(cols ...string) *Table {
	t := &Table{}
	for _, c := range cols {
		t.columns = append(t.columns, tableCol{Name: c})
	}
	return t
}

// Col adds a column with explicit width. Chainable.
func (t *Table) Col(name string, width int) *Table {
	t.columns = append(t.columns, tableCol{Name: name, Width: width})
	return t
}

// Row adds a row of values (must match column count).
func (t *Table) Row(vals ...string) {
	t.rows = append(t.rows, vals)
}

// Len returns number of rows.
func (t *Table) Len() int {
	return len(t.rows)
}

// Render outputs the table in the given mode.
func (t *Table) Render(mode OutputMode) {
	switch mode {
	case ModeJSON:
		t.renderJSON()
	case ModeWide:
		t.renderWide()
	default:
		t.renderText()
	}
}

// RenderWithTotal adds "N items" footer (unless quietMode).
func (t *Table) RenderWithTotal(mode OutputMode) {
	t.Render(mode)
	if mode != ModeJSON && !quietMode {
		fmt.Printf("%d items\n", len(t.rows))
	}
}

func (t *Table) renderJSON() {
	items := make([]map[string]string, 0, len(t.rows))
	for _, row := range t.rows {
		obj := make(map[string]string, len(t.columns))
		for i, col := range t.columns {
			if i < len(row) {
				obj[col.Name] = row[i]
			}
		}
		items = append(items, obj)
	}
	out := map[string]interface{}{"items": items, "total": len(items)}
	data, _ := json.Marshal(out)
	fmt.Println(string(data))
}

func (t *Table) renderWide() {
	widths := t.computeWidths()
	if len(t.rows) > 0 {
		// Header
		for i, col := range t.columns {
			if i > 0 {
				fmt.Print("  ")
			}
			fmt.Printf("%-*s", widths[i], strings.ToUpper(col.Name))
		}
		fmt.Println()
		// Data
		for _, row := range t.rows {
			for i, col := range t.columns {
				val := ""
				if i < len(row) {
					val = row[i]
				}
				if i > 0 {
					fmt.Print("  ")
				}
				w := widths[i]
				// Last column: no padding
				if i == len(t.columns)-1 {
					fmt.Print(val)
				} else {
					if len(val) > w {
						val = val[:w-1] + "…"
					}
					fmt.Printf("%-*s", w, val)
				}
				_ = col // used above
			}
			fmt.Println()
		}
	}
}

func (t *Table) renderText() {
	for _, row := range t.rows {
		fmt.Println(strings.Join(row, "\t"))
	}
}

// computeWidths returns final column widths for wide mode.
func (t *Table) computeWidths() []int {
	widths := make([]int, len(t.columns))
	for i, col := range t.columns {
		if col.Width > 0 {
			widths[i] = col.Width
			continue
		}
		// Check known field widths first
		if w := fieldWidth(col.Name); w != 16 || isKnownField(col.Name) {
			widths[i] = w
			continue
		}
		// Auto-compute from data: max(header, data values), clamped 8..50
		maxW := len(col.Name)
		for _, row := range t.rows {
			if i < len(row) && len(row[i]) > maxW {
				maxW = len(row[i])
			}
		}
		if maxW < 8 {
			maxW = 8
		}
		if maxW > 50 {
			maxW = 50
		}
		widths[i] = maxW + 1 // +1 for breathing room
	}
	return widths
}

// isKnownField returns true if fieldWidth returns a real value (not the 16 default).
func isKnownField(name string) bool {
	switch name {
	case "id", "title", "path", "type", "status", "tags",
		"updated", "created", "tier", "confidence", "origin", "basis":
		return true
	}
	return false
}
