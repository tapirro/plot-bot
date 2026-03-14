package main

import (
	"testing"
	"time"
)

func testArtifacts() []Artifact {
	return []Artifact{
		{ID: "pat-auth", Type: "pattern", Status: "active", Confidence: "validated", Origin: "harvest/hilart", Tags: []string{"security", "auth"}, Path: "knowledge/playbook/pat_auth.md", Updated: "2026-03-10", Tier: 2, Title: "Auth Pattern"},
		{ID: "pat-cache", Type: "pattern", Status: "draft", Confidence: "hypothesis", Origin: "harvest/voic", Tags: []string{"perf"}, Path: "knowledge/playbook/pat_cache.md", Updated: "2026-03-08", Tier: 1, Title: "Cache Pattern"},
		{ID: "meth-quality", Type: "methodology", Status: "active", Confidence: "validated", Origin: "synthesis", Tags: []string{"quality"}, Path: "knowledge/playbook/meth_quality.md", Updated: "2026-03-12", Tier: 2, Title: "Quality Methodology"},
		{ID: "ins-meeting", Type: "insight", Status: "imported", Confidence: "", Origin: "", Tags: []string{"meeting"}, Path: "work/intake/ins_meeting.md", Updated: "2026-01-01", Tier: 0, Title: "Meeting Notes"},
		{ID: "bet-hive", Type: "bet", Status: "active", Confidence: "", Origin: "", Tags: []string{"infra", "auth"}, Path: "work/bets/hive.md", Updated: "2026-03-11", Tier: 1, Title: "Hive Master Bet"},
	}
}

func TestPipelineTypeFilter(t *testing.T) {
	items := Query(testArtifacts()).Type("pattern").Items()
	if len(items) != 2 {
		t.Fatalf("expected 2 patterns, got %d", len(items))
	}
	for _, a := range items {
		if a.Type != "pattern" {
			t.Errorf("expected type=pattern, got %s", a.Type)
		}
	}
}

func TestPipelineTypeComma(t *testing.T) {
	items := Query(testArtifacts()).Type("pattern,methodology").Items()
	if len(items) != 3 {
		t.Fatalf("expected 3 items (2 pattern + 1 methodology), got %d", len(items))
	}
}

func TestPipelineTypeCaseInsensitive(t *testing.T) {
	items := Query(testArtifacts()).Type("Pattern").Items()
	if len(items) != 2 {
		t.Fatalf("expected 2 patterns (case-insensitive), got %d", len(items))
	}
}

func TestPipelineStatusFilter(t *testing.T) {
	items := Query(testArtifacts()).Status("active").Items()
	if len(items) != 3 {
		t.Fatalf("expected 3 active, got %d", len(items))
	}
}

func TestPipelineChainTypeAndStatus(t *testing.T) {
	items := Query(testArtifacts()).Type("pattern").Status("active").Items()
	if len(items) != 1 {
		t.Fatalf("expected 1 active pattern, got %d", len(items))
	}
	if items[0].ID != "pat-auth" {
		t.Errorf("expected pat-auth, got %s", items[0].ID)
	}
}

func TestPipelineConfidenceFilter(t *testing.T) {
	items := Query(testArtifacts()).Confidence("validated").Items()
	if len(items) != 2 {
		t.Fatalf("expected 2 validated, got %d", len(items))
	}
}

func TestPipelineConfidenceCaseInsensitive(t *testing.T) {
	items := Query(testArtifacts()).Confidence("Validated").Items()
	if len(items) != 2 {
		t.Fatalf("expected 2 validated (case-insensitive), got %d", len(items))
	}
}

func TestPipelineOriginExact(t *testing.T) {
	items := Query(testArtifacts()).Origin("synthesis").Items()
	if len(items) != 1 {
		t.Fatalf("expected 1 with origin=synthesis, got %d", len(items))
	}
	if items[0].ID != "meth-quality" {
		t.Errorf("expected meth-quality, got %s", items[0].ID)
	}
}

func TestPipelineOriginPrefix(t *testing.T) {
	items := Query(testArtifacts()).Origin("harvest").Items()
	if len(items) != 2 {
		t.Fatalf("expected 2 with origin prefix=harvest, got %d", len(items))
	}
}

func TestPipelineZoneFilter(t *testing.T) {
	items := Query(testArtifacts()).Zone("knowledge/").Items()
	if len(items) != 3 {
		t.Fatalf("expected 3 in knowledge/ zone, got %d", len(items))
	}
}

func TestPipelineTagFilter(t *testing.T) {
	items := Query(testArtifacts()).Tag("auth").Items()
	if len(items) != 2 {
		t.Fatalf("expected 2 with tag=auth, got %d", len(items))
	}
}

func TestPipelineTagMultiple(t *testing.T) {
	// Require both tags
	items := Query(testArtifacts()).Tag("infra,auth").Items()
	if len(items) != 1 {
		t.Fatalf("expected 1 with both infra+auth, got %d", len(items))
	}
	if items[0].ID != "bet-hive" {
		t.Errorf("expected bet-hive, got %s", items[0].ID)
	}
}

func TestPipelineWherePredicate(t *testing.T) {
	preds := parseWhere("tier>1")
	items := Query(testArtifacts()).Where(preds).Items()
	if len(items) != 2 {
		t.Fatalf("expected 2 with tier>1, got %d", len(items))
	}
}

func TestPipelineWhereNil(t *testing.T) {
	all := testArtifacts()
	items := Query(all).Where(nil).Items()
	if len(items) != len(all) {
		t.Fatalf("expected %d (nil where = no-op), got %d", len(all), len(items))
	}
}

func TestPipelineSortAndLimit(t *testing.T) {
	items := Query(testArtifacts()).Sort("updated").Limit(2).Items()
	if len(items) != 2 {
		t.Fatalf("expected 2, got %d", len(items))
	}
	// Sort ascending by updated — oldest first
	if items[0].Updated > items[1].Updated {
		t.Errorf("expected ascending sort, got %s > %s", items[0].Updated, items[1].Updated)
	}
}

func TestPipelineSortDefault(t *testing.T) {
	items := Query(testArtifacts()).SortDefault().Items()
	if len(items) < 2 {
		t.Fatal("expected at least 2 items")
	}
	// Default sort = newest first
	if items[0].Updated < items[1].Updated {
		t.Errorf("expected descending sort, got %s < %s", items[0].Updated, items[1].Updated)
	}
}

func TestPipelineLimitZeroIsNoop(t *testing.T) {
	all := testArtifacts()
	items := Query(all).Limit(0).Items()
	if len(items) != len(all) {
		t.Fatalf("expected %d (limit 0 = no-op), got %d", len(all), len(items))
	}
}

func TestPipelineLimitExceedsLength(t *testing.T) {
	all := testArtifacts()
	items := Query(all).Limit(100).Items()
	if len(items) != len(all) {
		t.Fatalf("expected %d (limit > len = no-op), got %d", len(all), len(items))
	}
}

func TestPipelineGapFilter(t *testing.T) {
	// Initialize type rules needed by requiredTierFor
	initRules()
	initTypes()

	items := Query(testArtifacts()).Gap().Items()
	// All items with tier < required should be returned
	for _, a := range items {
		if a.Tier >= requiredTierFor(a) {
			t.Errorf("gap filter passed artifact %s with tier=%d >= required=%d", a.ID, a.Tier, requiredTierFor(a))
		}
	}
}

func TestPipelineStaleFilter(t *testing.T) {
	arts := []Artifact{
		{ID: "fresh", Status: "active", Updated: time.Now().Format("2006-01-02")},
		{ID: "old-draft", Status: "draft", Updated: time.Now().AddDate(0, 0, -20).Format("2006-01-02")},
		{ID: "ancient", Status: "active", Updated: "2025-01-01"},
	}
	items := Query(arts).Stale().Items()
	if len(items) != 2 {
		t.Fatalf("expected 2 stale (old-draft + ancient), got %d", len(items))
	}
}

func TestPipelineMinTierNegativeIsNoop(t *testing.T) {
	all := testArtifacts()
	items := Query(all).MinTier(-1).Items()
	if len(items) != len(all) {
		t.Fatalf("expected %d (MinTier -1 = no-op), got %d", len(all), len(items))
	}
}

func TestPipelineMinTier(t *testing.T) {
	items := Query(testArtifacts()).MinTier(2).Items()
	for _, a := range items {
		if a.Tier != 2 {
			t.Errorf("expected tier=2, got %d for %s", a.Tier, a.ID)
		}
	}
}

func TestPipelineCount(t *testing.T) {
	c := Query(testArtifacts()).Type("pattern").Count()
	if c != 2 {
		t.Fatalf("expected count=2, got %d", c)
	}
}

func TestPipelineEmptyFiltersAreNoops(t *testing.T) {
	all := testArtifacts()
	items := Query(all).
		Type("").
		Status("").
		Tag("").
		Confidence("").
		Origin("").
		Zone("").
		Where(nil).
		Past("").
		HasSection("").
		ContentMatch("").
		TextSearch("").
		Sort("").
		Limit(0).
		Items()
	if len(items) != len(all) {
		t.Fatalf("expected %d (all empty = no-op), got %d", len(all), len(items))
	}
}

func TestPipelineNilFilter(t *testing.T) {
	all := testArtifacts()
	items := Query(all).Filter(nil).Items()
	if len(items) != len(all) {
		t.Fatalf("expected %d (nil filter = no-op), got %d", len(all), len(items))
	}
}

func TestPipelineEmptyInput(t *testing.T) {
	items := Query(nil).Type("pattern").Status("active").Items()
	if len(items) != 0 {
		t.Fatalf("expected 0 from nil input, got %d", len(items))
	}
	if Query(nil).Count() != 0 {
		t.Fatal("expected count=0 from nil input")
	}
}

func TestPipelineTextSearch(t *testing.T) {
	items := Query(testArtifacts()).TextSearch("hive").Items()
	if len(items) != 1 {
		t.Fatalf("expected 1 match for 'hive', got %d", len(items))
	}
	if items[0].ID != "bet-hive" {
		t.Errorf("expected bet-hive, got %s", items[0].ID)
	}
}

func TestPipelinePastFilter(t *testing.T) {
	items := Query(testArtifacts()).Past("0311").Items()
	// Only items updated >= 2026-03-11
	for _, a := range items {
		if a.Updated < "2026-03-11" {
			t.Errorf("expected updated >= 2026-03-11, got %s for %s", a.Updated, a.ID)
		}
	}
}

func TestPipelineComplexChain(t *testing.T) {
	// Pattern + active + origin prefix + sorted by id + limit 1
	items := Query(testArtifacts()).
		Type("pattern").
		Status("active").
		Origin("harvest").
		Sort("id").
		Limit(1).
		Items()
	if len(items) != 1 {
		t.Fatalf("expected 1 from complex chain, got %d", len(items))
	}
	if items[0].ID != "pat-auth" {
		t.Errorf("expected pat-auth, got %s", items[0].ID)
	}
}
