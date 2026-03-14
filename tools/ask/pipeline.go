package main

import (
	"sort"
	"strings"
	"time"
)

// Pipeline provides a fluent interface for filtering, sorting, and rendering artifacts.
type Pipeline struct {
	items []Artifact
}

// Query starts a pipeline from a slice of artifacts.
func Query(items []Artifact) *Pipeline {
	return &Pipeline{items: items}
}

// Filter applies a predicate. No-op if pred is nil.
func (p *Pipeline) Filter(pred func(Artifact) bool) *Pipeline {
	if pred == nil {
		return p
	}
	p.items = filterArtifacts(p.items, pred)
	return p
}

// Type filters by type (comma-separated, case-insensitive). No-op if empty.
func (p *Pipeline) Type(typeSpec string) *Pipeline {
	if typeSpec == "" {
		return p
	}
	types := map[string]bool{}
	for _, t := range strings.Split(typeSpec, ",") {
		types[strings.TrimSpace(strings.ToLower(t))] = true
	}
	p.items = filterArtifacts(p.items, func(a Artifact) bool {
		return types[strings.ToLower(a.Type)]
	})
	return p
}

// Status filters by status (case-insensitive). No-op if empty.
func (p *Pipeline) Status(status string) *Pipeline {
	if status == "" {
		return p
	}
	p.items = filterArtifacts(p.items, func(a Artifact) bool {
		return strings.EqualFold(a.Status, status)
	})
	return p
}

// Confidence filters by exact confidence match (case-insensitive). No-op if empty.
func (p *Pipeline) Confidence(conf string) *Pipeline {
	if conf == "" {
		return p
	}
	p.items = filterArtifacts(p.items, func(a Artifact) bool {
		return strings.EqualFold(a.Confidence, conf)
	})
	return p
}

// Origin filters by origin prefix match. No-op if empty.
func (p *Pipeline) Origin(origin string) *Pipeline {
	if origin == "" {
		return p
	}
	p.items = filterArtifacts(p.items, func(a Artifact) bool {
		return strings.EqualFold(a.Origin, origin) ||
			strings.HasPrefix(strings.ToLower(a.Origin), strings.ToLower(origin)+"/")
	})
	return p
}

// Zone filters by path prefix. No-op if empty.
func (p *Pipeline) Zone(zone string) *Pipeline {
	if zone == "" {
		return p
	}
	p.items = filterArtifacts(p.items, func(a Artifact) bool {
		return strings.HasPrefix(a.Path, zone)
	})
	return p
}

// Tag filters artifacts that have ALL specified tags (comma-separated). No-op if empty.
func (p *Pipeline) Tag(tagSpec string) *Pipeline {
	if tagSpec == "" {
		return p
	}
	required := strings.Split(strings.ToLower(tagSpec), ",")
	p.items = filterArtifacts(p.items, func(a Artifact) bool {
		for _, req := range required {
			req = strings.TrimSpace(req)
			if req == "" {
				continue
			}
			found := false
			for _, t := range a.Tags {
				if strings.ToLower(t) == req {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	})
	return p
}

// Where applies parsed WHERE predicates. No-op if empty.
func (p *Pipeline) Where(preds []Predicate) *Pipeline {
	if len(preds) == 0 {
		return p
	}
	p.items = filterWhere(p.items, preds)
	return p
}

// Past filters to artifacts updated after date. No-op if empty.
func (p *Pipeline) Past(dateStr string) *Pipeline {
	if dateStr == "" {
		return p
	}
	after := parseShortDate(dateStr)
	if after == "" {
		return p
	}
	p.items = filterArtifacts(p.items, func(a Artifact) bool {
		return a.Updated >= after
	})
	return p
}

// Stale filters to artifacts older than status-based thresholds (draft>14d, imported>90d, any>180d).
func (p *Pipeline) Stale() *Pipeline {
	now := time.Now()
	p.items = filterArtifacts(p.items, func(a Artifact) bool {
		d, err := time.Parse("2006-01-02", a.Updated)
		if err != nil {
			return true
		}
		age := int(now.Sub(d).Hours() / 24)
		if a.Status == "draft" && age > 14 {
			return true
		} else if a.Status == "imported" && age > 90 {
			return true
		} else if age > 180 {
			return true
		}
		return false
	})
	return p
}

// MinTier filters to artifacts at exactly the given tier. No-op if negative.
func (p *Pipeline) MinTier(tier int) *Pipeline {
	if tier < 0 {
		return p
	}
	p.items = filterArtifacts(p.items, func(a Artifact) bool {
		return a.Tier == tier
	})
	return p
}

// Gap filters to artifacts below their required tier.
func (p *Pipeline) Gap() *Pipeline {
	p.items = filterArtifacts(p.items, func(a Artifact) bool {
		return a.Tier < requiredTierFor(a)
	})
	return p
}

// HasSection filters to artifacts containing a section matching the slug. No-op if empty.
func (p *Pipeline) HasSection(slug string) *Pipeline {
	if slug == "" {
		return p
	}
	p.items = filterHasSection(p.items, slug)
	return p
}

// ContentMatch filters by file content regex. No-op if empty. Expensive — apply last.
func (p *Pipeline) ContentMatch(pattern string) *Pipeline {
	if pattern == "" {
		return p
	}
	p.items = filterContentMatch(p.items, pattern)
	return p
}

// TextSearch filters by text match across id, title, path, tags. No-op if empty.
func (p *Pipeline) TextSearch(q string) *Pipeline {
	if q == "" {
		return p
	}
	lower := strings.ToLower(q)
	p.items = filterArtifacts(p.items, func(a Artifact) bool {
		text := strings.ToLower(a.ID + " " + a.Title + " " + a.Path + " " + strings.Join(a.Tags, " "))
		return strings.Contains(text, lower)
	})
	return p
}

// Sort sorts by field name. No-op if empty. Sorts by updated desc by default when
// called without arguments — use SortDefault for that.
func (p *Pipeline) Sort(field string) *Pipeline {
	if field == "" {
		return p
	}
	sort.Slice(p.items, func(i, j int) bool {
		return fieldValue(p.items[i], field) < fieldValue(p.items[j], field)
	})
	return p
}

// SortDefault sorts by updated descending (newest first).
func (p *Pipeline) SortDefault() *Pipeline {
	sort.Slice(p.items, func(i, j int) bool {
		return p.items[i].Updated > p.items[j].Updated
	})
	return p
}

// Limit caps the result to N items. No-op if 0 or negative.
func (p *Pipeline) Limit(n int) *Pipeline {
	if n <= 0 {
		return p
	}
	if n < len(p.items) {
		p.items = p.items[:n]
	}
	return p
}

// Items returns the filtered artifacts.
func (p *Pipeline) Items() []Artifact {
	return p.items
}

// Count returns the count.
func (p *Pipeline) Count() int {
	return len(p.items)
}
