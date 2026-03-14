package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// --- Error types ---

// UsageError indicates incorrect command usage (wrong args, missing flags).
type UsageError struct{ Msg string }

func (e *UsageError) Error() string { return e.Msg }

// NotFoundError indicates a missing artifact, action, or resource.
type NotFoundError struct{ What string }

func (e *NotFoundError) Error() string { return fmt.Sprintf("not found: %s", e.What) }

// FileError indicates a file I/O failure.
type FileError struct {
	Path string
	Err  error
}

func (e *FileError) Error() string { return fmt.Sprintf("cannot read %s: %v", e.Path, e.Err) }

// --- Helpers ---

type KV struct {
	Key   string
	Value int
}

func sortMapByValue(m map[string]int) []KV {
	var kvs []KV
	for k, v := range m {
		kvs = append(kvs, KV{k, v})
	}
	sort.Slice(kvs, func(i, j int) bool {
		if kvs[i].Value != kvs[j].Value {
			return kvs[i].Value > kvs[j].Value
		}
		return kvs[i].Key < kvs[j].Key
	})
	return kvs
}

func sortMapByKey(m map[string]int) []KV {
	var kvs []KV
	for k, v := range m {
		kvs = append(kvs, KV{k, v})
	}
	sort.Slice(kvs, func(i, j int) bool {
		return kvs[i].Key < kvs[j].Key
	})
	return kvs
}

func sortMapByKey2(m map[string][4]int) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func countTierAbove(items []Artifact, minTier int) int {
	n := 0
	for _, a := range items {
		if a.Tier >= minTier {
			n++
		}
	}
	return n
}

func pctTierAbove(items []Artifact, minTier int) int {
	if len(items) == 0 {
		return 0
	}
	return countTierAbove(items, minTier) * 100 / len(items)
}

func parseShortDate(s string) string {
	if len(s) == 4 {
		return fmt.Sprintf("%d-%s-%s", time.Now().Year(), s[:2], s[2:])
	}
	return s
}

func filterArtifacts(items []Artifact, pred func(Artifact) bool) []Artifact {
	var result []Artifact
	for _, a := range items {
		if pred(a) {
			result = append(result, a)
		}
	}
	return result
}

func findByID(idx *Index, id string) *Artifact {
	for i := range idx.Artifacts {
		if idx.Artifacts[i].ID == id {
			return &idx.Artifacts[i]
		}
	}
	return nil
}

func isActionsHeading(trimmed string) bool {
	return strings.HasPrefix(trimmed, "## Actions") || strings.HasPrefix(trimmed, "## Действия")
}

func jsonPrintCompact(v interface{}) {
	data, _ := json.Marshal(v)
	fmt.Println(string(data))
}

func jsonPrint(v interface{}) {
	data, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(data))
}

func compliancePct(n, total int) int {
	if total == 0 {
		return 0
	}
	return n * 100 / total
}
