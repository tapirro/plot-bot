package main

import (
	_ "embed"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
)

// --- Embedded schema ---

//go:embed schema.json
var schemaData []byte

type TypeDef struct {
	Group      string   `json:"group"`
	Statuses   []string `json:"statuses"`
	Tier       int      `json:"tier"`
	Confidence string   `json:"confidence"`
}

type SchemaConfig struct {
	Types            map[string]TypeDef `json:"types"`
	ConfidenceLevels []string           `json:"confidence_levels"`
	Scan             struct {
		Zones      []string `json:"zones"`
		Extensions []string `json:"extensions"`
	} `json:"scan"`
	ZeroTierZones []string `json:"zero_tier_zones"`
}

var schemaConfig SchemaConfig

func init() {
	if err := json.Unmarshal(schemaData, &schemaConfig); err != nil {
		panic("invalid schema.json: " + err.Error())
	}
	buildDerivedMaps()
}

// --- Core types ---

type Artifact struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Status   string   `json:"status"`
	Domain   string   `json:"domain,omitempty"`
	Tags     []string `json:"tags"`
	Title    string   `json:"title"`
	Path     string   `json:"path"`
	Updated  string   `json:"updated"`
	Created  string   `json:"created"`
	Sections []string `json:"sections"`
	EdgesOut []string `json:"edges_out"`
	Source     string   `json:"source,omitempty"`
	Tier       int      `json:"tier"`
	Confidence string   `json:"confidence,omitempty"`
	Origin     string   `json:"origin,omitempty"`
	Basis      string   `json:"basis,omitempty"`
}

type Index struct {
	Artifacts []Artifact `json:"artifacts"`
	Meta      IndexMeta  `json:"_meta"`
}

type IndexMeta struct {
	Updated   string `json:"updated"`
	Total     int    `json:"total"`
	GitOnly   bool   `json:"git_only,omitempty"`
	Generator string `json:"generator"`
}

// --- Path inference rules ---

type PathRule struct {
	Pattern string
	Type    string
	Status  string
	Tags    []string
}

var pathRules = []PathRule{
	{"^knowledge/hilart/", "reference", "imported", []string{"hilart"}},
	{"^knowledge/playbook/pattern_", "pattern", "active", nil},
	{"^knowledge/playbook/checklist_", "checklist", "active", nil},
	{"^knowledge/playbook/", "methodology", "active", nil},
	{"^knowledge/transcripts/insights/", "insight", "draft", nil},
	{"^knowledge/transcripts/digests/", "digest", "draft", nil},
	{"^knowledge/transcripts/\\d{4}-\\d{2}-\\d{2}_", "transcript", "raw", nil},
	{"^knowledge/concepts/", "guide", "active", nil},
	{"^knowledge/", "guide", "active", nil},
	{"^out/reports/", "report", "draft", nil},
	{"^out/deliverables/cross_agent/patterns/", "pattern", "active", []string{"cross-agent"}},
	{"^out/deliverables/cross_agent/methodologies/", "methodology", "active", []string{"cross-agent"}},
	{"^out/deliverables/cross_agent/best_practices/", "methodology", "active", []string{"cross-agent"}},
	{"^out/deliverables/", "report", "final", nil},
	{"^work/intake/deals/", "deal", "intake", nil},
	{"^work/topics/.*/TOPIC\\.md$", "topic", "open", nil},
	{"^work/topics/", "topic", "open", nil},
	{"^work/projects/", "project", "active", nil},
	{"^work/intake/harvests/", "report", "draft", nil},
	{"^work/", "guide", "active", nil},
	{"^tools/scripts/", "script", "active", nil},
	{"^tools/templates/", "template", "active", nil},
	{"^tools/", "guide", "active", nil},
	{"^in/", "guide", "imported", nil},
}

var compiledRules []struct {
	Re     *regexp.Regexp
	Type   string
	Status string
	Tags   []string
}

func initRules() {
	for _, r := range pathRules {
		compiledRules = append(compiledRules, struct {
			Re     *regexp.Regexp
			Type   string
			Status string
			Tags   []string
		}{regexp.MustCompile(r.Pattern), r.Type, r.Status, r.Tags})
	}
}

func inferFromPath(relPath string) (typ, status string, tags []string) {
	for _, r := range compiledRules {
		if r.Re.MatchString(relPath) {
			return r.Type, r.Status, r.Tags
		}
	}
	return "guide", "active", nil
}

// --- Type registry (populated from schema.json via buildDerivedMaps) ---

var typeGroups map[string][]string
var knownTypes map[string]bool
var validStatuses map[string][]string
var validConfidence map[string]bool
var confidenceRequired map[string]bool
var confidenceRecommended map[string]bool
var requiredTier map[string]int
var zeroTierZones []string

func initTypes() {
	// no-op: types are now built from schema.json in init()
}

func buildDerivedMaps() {
	// typeGroups: group → []type (uppercased group key for backward compat)
	typeGroups = make(map[string][]string)
	for typeName, td := range schemaConfig.Types {
		key := strings.ToUpper(td.Group)
		typeGroups[key] = append(typeGroups[key], typeName)
	}

	// knownTypes
	knownTypes = make(map[string]bool)
	for typeName := range schemaConfig.Types {
		knownTypes[typeName] = true
	}

	// validStatuses
	validStatuses = make(map[string][]string)
	for typeName, td := range schemaConfig.Types {
		validStatuses[typeName] = td.Statuses
	}

	// validConfidence
	validConfidence = make(map[string]bool)
	for _, c := range schemaConfig.ConfidenceLevels {
		validConfidence[c] = true
	}

	// confidenceRequired, confidenceRecommended
	confidenceRequired = make(map[string]bool)
	confidenceRecommended = make(map[string]bool)
	for typeName, td := range schemaConfig.Types {
		switch td.Confidence {
		case "required":
			confidenceRequired[typeName] = true
		case "recommended":
			confidenceRecommended[typeName] = true
		}
	}

	// requiredTier
	requiredTier = make(map[string]int)
	for typeName, td := range schemaConfig.Types {
		requiredTier[typeName] = td.Tier
	}

	// zeroTierZones
	zeroTierZones = schemaConfig.ZeroTierZones

	// scanZones, scanExtensions
	scanZones = schemaConfig.Scan.Zones
	scanExtensions = make(map[string]bool)
	for _, ext := range schemaConfig.Scan.Extensions {
		scanExtensions[ext] = true
	}
}

func requiredTierFor(a Artifact) int {
	for _, z := range zeroTierZones {
		if strings.HasPrefix(a.Path, z) {
			return 0
		}
	}
	if t, ok := requiredTier[a.Type]; ok {
		return t
	}
	return 1
}

// --- Domain inference ---

type DomainRule struct {
	Pattern string
	Domain  string
}

var domainRules = []DomainRule{
	// Hilart subdomains
	{"hilart.*/resale|resale_identif", "hilart/resale"},
	{"dtk[_-]|dtk/|warehouse[_-]|leads[_-]|order.state", "hilart/ops"},
	{"cpa[_-]|cpa/|cost.per.acq", "hilart/cpa"},
	{"hilart.*/dev|ops.bot|hilart.*bot", "hilart/dev"},
	{"pnl|unit.econom|revenue|margin|cohort|analyt.*hilart", "hilart/dwh"},
	{"hilart", "hilart"},
	// Voic
	{"voic.*research|speech|tts|stt|whisper|diariza", "voic/research"},
	{"voic.*app|voic.*web", "voic/app"},
	{"voic.*bot|voic.*sell", "voic/bot-seller"},
	{"voic", "voic"},
	// Hive
	{"hive.*infra|spora.*infra|bootstrap|agent.json", "hive/infra"},
	{"hive.*network|agent.*directory|inter.agent", "hive/network"},
	{"hive|spora", "hive"},
	// Assistant subdomains
	{"analytics|session.*xray|token.econom|claude.log|etl|cost.model", "assistant/analytics"},
	{"design.system|observatory|base\\.css|mantissa.*design", "assistant/design"},
	{"tools/scripts/|tools/templates/", "assistant/tooling"},
	{"awr|concept\\.md|workshop|intake.*triage|domain.*charter", "assistant/awr"},
	// Corp-dev
	{"bible|stories/|narrative", "corp-dev/bible"},
	{"governance|charter.*company|policy.*company", "corp-dev/governance"},
	{"strategy|vera|meta.cycle|bet[_-]|deliberat", "corp-dev/strategy"},
	// Real estate
	{"batumi|adjara|gonio|chakvi|kobuleti", "real-estate/batumi"},
	{"real.estate|napr|cadastr|deal[_-]", "real-estate"},
	// Cross-agent
	{"cross.agent.*pattern|distill.*pattern", "cross-agent/patterns"},
	{"cross.agent.*package|plugin|deliverable.*package", "cross-agent/packages"},
	{"cross.agent.*method|cross.agent.*best.practice", "cross-agent/methodologies"},
	{"cross.agent", "cross-agent"},
	// Distillat
	{"distillat", "distillat"},
}

var compiledDomainRules []struct {
	Re     *regexp.Regexp
	Domain string
}

func initDomainRules() {
	for _, r := range domainRules {
		compiledDomainRules = append(compiledDomainRules, struct {
			Re     *regexp.Regexp
			Domain string
		}{regexp.MustCompile("(?i)" + r.Pattern), r.Domain})
	}
}

func inferDomain(relPath string) string {
	lower := strings.ToLower(relPath)
	for _, r := range compiledDomainRules {
		if r.Re.MatchString(lower) {
			return r.Domain
		}
	}
	return ""
}

// --- Scan config (populated from schema.json via buildDerivedMaps) ---

var scanZones []string
var scanExtensions map[string]bool

// --- ID generation ---

var datePrefix = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[_-]`)
var datePrefixShort = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})[_-]`)

func generateID(relPath, typ string) string {
	basename := strings.TrimSuffix(filepath.Base(relPath), filepath.Ext(relPath))

	dateTag := ""
	if m := datePrefixShort.FindStringSubmatch(basename); m != nil {
		dateTag = m[2] + m[3]
	}

	name := datePrefix.ReplaceAllString(basename, "")
	name = strings.ToLower(strings.ReplaceAll(name, "_", "-"))

	dir := filepath.Dir(relPath)
	parts := strings.Split(dir, string(filepath.Separator))
	if len(parts) > 1 {
		var dirParts []string
		for i := len(parts) - 1; i >= 1 && len(dirParts) < 2; i-- {
			p := strings.ToLower(strings.ReplaceAll(parts[i], "_", "-"))
			p = datePrefix.ReplaceAllString(p, "")
			if p != "" && p != "." {
				dirParts = append([]string{p}, dirParts...)
			}
		}
		if len(dirParts) > 0 {
			prefix := strings.Join(dirParts, "-")
			if !strings.Contains(name, dirParts[len(dirParts)-1]) {
				name = prefix + "-" + name
			}
		}
	}

	if len(name) > 50 {
		if i := strings.LastIndex(name[:50], "-"); i > 0 {
			name = name[:i]
		} else {
			name = name[:50]
		}
	}

	if dateTag != "" {
		name = name + "-" + dateTag
	}

	if strings.HasPrefix(name, typ+"-") || name == typ {
		return name
	}
	return typ + "-" + name
}
