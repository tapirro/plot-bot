package main

import (
	"path/filepath"
	"regexp"
	"strings"
)

// --- Frontmatter ---

func parseFrontmatter(content string) (map[string]interface{}, int) {
	if !strings.HasPrefix(content, "---") {
		return nil, 0
	}
	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return nil, 0
	}
	headerText := rest[:idx]
	bodyStart := 3 + idx + 4
	if bodyStart < len(content) && content[bodyStart] == '\n' {
		bodyStart++
	}
	fm := parseSimpleYAML(headerText)
	return fm, bodyStart
}

// parseSimpleYAML handles flat key-value frontmatter with inline lists.
// Zero external dependencies by design — frontmatter is always flat.
func parseSimpleYAML(text string) map[string]interface{} {
	result := make(map[string]interface{})
	for _, rawLine := range strings.Split(text, "\n") {
		if strings.HasPrefix(rawLine, "  ") || strings.HasPrefix(rawLine, "\t") {
			continue
		}
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		colonIdx := strings.Index(line, ":")
		if colonIdx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:colonIdx])
		val := strings.TrimSpace(line[colonIdx+1:])

		if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") {
			inner := val[1 : len(val)-1]
			var items []string
			for _, item := range strings.Split(inner, ",") {
				item = strings.TrimSpace(item)
				item = strings.Trim(item, "\"'")
				if item != "" {
					items = append(items, item)
				}
			}
			result[key] = items
			continue
		}

		if ci := strings.Index(val, " #"); ci >= 0 {
			val = strings.TrimSpace(val[:ci])
		}
		val = strings.Trim(val, "\"'")
		result[key] = val
	}
	return result
}

func fmString(fm map[string]interface{}, key string) string {
	if fm == nil {
		return ""
	}
	v, ok := fm[key]
	if !ok {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func fmStringList(fm map[string]interface{}, key string) []string {
	if fm == nil {
		return nil
	}
	v, ok := fm[key]
	if !ok {
		return nil
	}
	switch val := v.(type) {
	case []string:
		return val
	case string:
		if val == "" {
			return nil
		}
		var items []string
		for _, s := range strings.Split(val, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				items = append(items, s)
			}
		}
		return items
	}
	return nil
}

// --- Content parsing ---

func getTitle(content string, bodyStart int) string {
	body := content[bodyStart:]
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") && !strings.HasPrefix(trimmed, "## ") {
			return strings.TrimSpace(trimmed[2:])
		}
	}
	return ""
}

var slugRe = regexp.MustCompile(`[^\p{L}\p{N}-]+`)
var dashRe = regexp.MustCompile(`-+`)

func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	s = slugRe.ReplaceAllString(s, "-")
	s = dashRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

func getSections(content string, bodyStart int) []string {
	body := content[bodyStart:]
	var sections []string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "## ") {
			heading := strings.TrimSpace(line[3:])
			slug := slugify(heading)
			if slug != "" {
				sections = append(sections, slug)
			}
		}
	}
	return sections
}

func getSectionContent(content string, sectionSlug string, bodyStart int) string {
	body := content[bodyStart:]
	lines := strings.Split(body, "\n")
	capturing := false
	var result []string
	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			heading := strings.TrimSpace(line[3:])
			slug := slugify(heading)
			if slug == sectionSlug {
				capturing = true
				continue
			} else if capturing {
				break
			}
		} else if capturing {
			result = append(result, line)
		}
	}
	for len(result) > 0 && strings.TrimSpace(result[0]) == "" {
		result = result[1:]
	}
	for len(result) > 0 && strings.TrimSpace(result[len(result)-1]) == "" {
		result = result[:len(result)-1]
	}
	return strings.Join(result, "\n")
}

var linkRe = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+\.md(?:#[^)]*)?)\)`)

func getEdges(content string, filePath string, bodyStart int) []string {
	body := content[bodyStart:]
	matches := linkRe.FindAllStringSubmatch(body, -1)
	var edges []string
	seen := map[string]bool{}
	for _, m := range matches {
		linkPath := strings.Split(m[2], "#")[0]
		if strings.HasPrefix(linkPath, "http") {
			continue
		}
		dir := filepath.Dir(filePath)
		resolved := filepath.Clean(filepath.Join(dir, linkPath))
		if !seen[resolved] {
			edges = append(edges, resolved)
			seen[resolved] = true
		}
	}
	return edges
}
