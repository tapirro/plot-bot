package main

import (
	"path/filepath"
	"sort"
	"strings"
)

// resolveArtifact finds an artifact by ID (exact → substring → fuzzy).
// Returns nil, Artifact, or []Artifact for ambiguous matches.
func resolveArtifact(query string, items []Artifact) interface{} {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}

	// Exact match on ID
	for _, a := range items {
		if strings.ToLower(a.ID) == q {
			return a
		}
	}

	// Substring match on ID
	var matches []Artifact
	for _, a := range items {
		if strings.Contains(strings.ToLower(a.ID), q) {
			matches = append(matches, a)
		}
	}
	if len(matches) == 1 {
		return matches[0]
	}
	if len(matches) > 1 {
		return matches
	}

	// Fuzzy: score by term hits in id + title + filename + tags
	terms := strings.Fields(q)
	type scored struct {
		Score int
		A     Artifact
	}
	var results []scored
	for _, a := range items {
		text := strings.ToLower(a.ID + " " + a.Title + " " + filepath.Base(a.Path) + " " + strings.Join(a.Tags, " "))
		score := 0
		for _, t := range terms {
			if strings.Contains(text, t) {
				score++
			}
		}
		if score > 0 {
			results = append(results, scored{score, a})
		}
	}

	if len(results) == 0 {
		return nil
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	best := results[0].Score
	var top []Artifact
	for _, r := range results {
		if r.Score == best {
			top = append(top, r.A)
		}
	}

	if len(top) == 1 {
		return top[0]
	}
	if len(top) > 10 {
		top = top[:10]
	}
	return top
}

// tierDiag returns what's missing for the next tier.
func tierDiag(a Artifact) (req int, missing []string) {
	req = requiredTierFor(a)
	if a.Tier >= req {
		return req, nil
	}
	if a.Tier < 1 && req >= 1 {
		missing = append(missing, "id")
	}
	if a.Tier < 2 && req >= 2 {
		if a.Source == "" {
			missing = append(missing, "source")
		}
	}
	return req, missing
}
