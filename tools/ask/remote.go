package main

// remote.go — Load and query Telema2 cache (context/telema_cache.json).
// Provides live data enrichment for @bets, @todo, @progress views.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Telema cache structures (mirrors Telema2 PullResponse schema)
// ---------------------------------------------------------------------------

type TelemaCache struct {
	SyncedAt string         `json:"synced_at"`
	Domains  []CacheDomain  `json:"domains"`
	Goals    []CacheGoal    `json:"goals"`
	Tasks    []CacheTask    `json:"tasks"`
	Metrics  []CacheMetric  `json:"metrics"`
}

type CacheDomain struct {
	ID        string `json:"id"`
	Domain    string `json:"domain"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Lifecycle string `json:"lifecycle"`
	OwnerID   string `json:"owner_id"`
	OwnerName string `json:"owner_name"`
}

type CacheGoal struct {
	ID                string  `json:"id"`
	ScreenID          string  `json:"screen_id"`
	Domain            string  `json:"domain"`
	Name              string  `json:"name"`
	Type              string  `json:"type"`
	AwrID             string  `json:"awr_id"`
	Completed         int     `json:"completed"`
	Total             int     `json:"total"`
	Closed            bool    `json:"closed"`
	Hypothesis        string  `json:"hypothesis"`
	Tension           string  `json:"tension"`
	Horizon           string  `json:"horizon"`
	BetStatus         string  `json:"bet_status"`
	MeasurementActual *float64 `json:"measurement_actual"`
	MeasurementTarget *float64 `json:"measurement_target"`
	UpdatedAt         string  `json:"updated_at"`
}

type CacheTask struct {
	ID            string   `json:"id"`
	ScreenID      string   `json:"screen_id"`
	Domain        string   `json:"domain"`
	GoalID        *string  `json:"goal_id"`
	Title         string   `json:"title"`
	Status        string   `json:"status"`
	PriorityScore *float64 `json:"priority_score"`
	PriorityLevel *string  `json:"priority_level"`
	AssigneeID    *string  `json:"assignee_id"`
	AssigneeName  *string  `json:"assignee_name"`
	Due           *string  `json:"due"`
	UpdatedAt     string   `json:"updated_at"`
}

type CacheMetric struct {
	ID           string   `json:"id"`
	ScreenID     string   `json:"screen_id"`
	Domain       string   `json:"domain"`
	Name         string   `json:"name"`
	DisplayName  string   `json:"display_name"`
	Type         string   `json:"type"`
	Unit         string   `json:"unit"`
	CurrentValue *float64 `json:"current_value"`
	TargetValue  *float64 `json:"target_value"`
	SourceType   string   `json:"source_type"`
	UpdatedAt    string   `json:"updated_at"`
}

// ---------------------------------------------------------------------------
// Cache loading
// ---------------------------------------------------------------------------

var cachedTelema *TelemaCache

func cachePath() string {
	return filepath.Join(root, "context", "telema_cache.json")
}

// loadCache reads context/telema_cache.json. Returns nil if not found.
// Cached after first load within a session.
func loadCache() *TelemaCache {
	if cachedTelema != nil {
		return cachedTelema
	}
	data, err := os.ReadFile(cachePath())
	if err != nil {
		return nil
	}
	var cache TelemaCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil
	}
	cachedTelema = &cache
	return cachedTelema
}

// cacheAge returns how old the cache is, or -1 if missing.
func cacheAge() time.Duration {
	c := loadCache()
	if c == nil || c.SyncedAt == "" {
		return -1
	}
	t, err := time.Parse(time.RFC3339Nano, c.SyncedAt)
	if err != nil {
		// Try alternate format
		t, err = time.Parse("2006-01-02T15:04:05.999999Z", c.SyncedAt)
		if err != nil {
			return -1
		}
	}
	return time.Since(t)
}

// ---------------------------------------------------------------------------
// Lookups
// ---------------------------------------------------------------------------

// goalByAwrID finds a cached goal by its AWR artifact id.
func goalByAwrID(awrID string) *CacheGoal {
	c := loadCache()
	if c == nil {
		return nil
	}
	for i := range c.Goals {
		if c.Goals[i].AwrID == awrID {
			return &c.Goals[i]
		}
	}
	return nil
}

// tasksByDomain returns cached tasks for a domain (prefix match).
func tasksByDomain(domain string) []CacheTask {
	c := loadCache()
	if c == nil {
		return nil
	}
	var result []CacheTask
	for _, t := range c.Tasks {
		if t.Domain == domain || strings.HasPrefix(t.Domain, domain+"/") {
			result = append(result, t)
		}
	}
	return result
}

// metricsByDomain returns cached metrics for a domain.
func metricsByDomain(domain string) []CacheMetric {
	c := loadCache()
	if c == nil {
		return nil
	}
	var result []CacheMetric
	for _, m := range c.Metrics {
		if m.Domain == domain || strings.HasPrefix(m.Domain, domain+"/") {
			result = append(result, m)
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// Bet enrichment
// ---------------------------------------------------------------------------

// enrichBetInfo adds Telema2 live data to a betInfo.
// Returns true if cache data was found.
func enrichBetInfo(bi *betInfo) bool {
	g := goalByAwrID(bi.ID)
	if g == nil {
		return false
	}
	bi.TelemaID = g.ID

	// Task progress from Telema2 (tasks linked to the goal)
	if g.Total > 0 {
		bi.TasksTotal = g.Total
		bi.TasksDone = g.Completed
	}

	// Live measurement
	if g.MeasurementActual != nil {
		bi.MeasurementActual = g.MeasurementActual
	}
	if g.MeasurementTarget != nil {
		bi.MeasurementTarget = g.MeasurementTarget
	}

	return true
}

// ---------------------------------------------------------------------------
// @telema view — show cache status + task overview
// ---------------------------------------------------------------------------

func cmdTelema(args []string, mode OutputMode, idx *Index) {
	c := loadCache()
	if c == nil {
		fmt.Println("No telema cache found. Run: tl sync pull -o context/telema_cache.json")
		return
	}

	age := cacheAge()
	ageStr := "?"
	if age >= 0 {
		if age < time.Hour {
			ageStr = fmt.Sprintf("%dm ago", int(age.Minutes()))
		} else if age < 24*time.Hour {
			ageStr = fmt.Sprintf("%dh ago", int(age.Hours()))
		} else {
			ageStr = fmt.Sprintf("%dd ago", int(age.Hours()/24))
		}
	}

	if mode == ModeJSON {
		out := map[string]interface{}{
			"synced_at":     c.SyncedAt,
			"age":           ageStr,
			"domains_count": len(c.Domains),
			"goals_count":   len(c.Goals),
			"tasks_count":   len(c.Tasks),
			"metrics_count": len(c.Metrics),
			"tasks":         c.Tasks,
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
		return
	}

	stale := ""
	if age > 4*time.Hour {
		stale = " ⚠ STALE"
	}
	fmt.Printf("TELEMA CACHE — synced %s%s\n", ageStr, stale)
	fmt.Printf("  %d domains · %d goals · %d tasks · %d metrics\n\n", len(c.Domains), len(c.Goals), len(c.Tasks), len(c.Metrics))

	// Tasks by domain
	type domTasks struct {
		domain string
		tasks  []CacheTask
	}
	byDomain := map[string][]CacheTask{}
	for _, t := range c.Tasks {
		byDomain[t.Domain] = append(byDomain[t.Domain], t)
	}
	var domains []domTasks
	for d, ts := range byDomain {
		domains = append(domains, domTasks{d, ts})
	}
	sort.Slice(domains, func(i, j int) bool {
		return len(domains[i].tasks) > len(domains[j].tasks)
	})

	for _, dt := range domains {
		fmt.Printf("  %s (%d tasks)\n", dt.domain, len(dt.tasks))
		// Show top 5 by priority
		sort.Slice(dt.tasks, func(i, j int) bool {
			si := dt.tasks[i].PriorityScore
			sj := dt.tasks[j].PriorityScore
			if si == nil {
				return false
			}
			if sj == nil {
				return true
			}
			return *si > *sj
		})
		limit := 5
		if limit > len(dt.tasks) {
			limit = len(dt.tasks)
		}
		for _, t := range dt.tasks[:limit] {
			prio := "  "
			if t.PriorityLevel != nil {
				prio = *t.PriorityLevel
			}
			assignee := ""
			if t.AssigneeName != nil {
				assignee = " @" + *t.AssigneeName
			}
			title := t.Title
			if len(title) > 50 {
				title = title[:47] + "..."
			}
			fmt.Printf("    %s %-8s %-50s%s\n", prio, t.Status, title, assignee)
		}
		if len(dt.tasks) > limit {
			fmt.Printf("    ... +%d more\n", len(dt.tasks)-limit)
		}
	}
}
