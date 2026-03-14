package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// cmdContract: computed agent slot contract from repo state
func cmdContract(args []string, mode OutputMode, idx *Index) {
	// --- Agent identity from .claude/agent.json ---
	agentName := "unknown"
	agentDesc := ""
	agentFile := filepath.Join(root, ".claude", "agent.json")
	if data, err := os.ReadFile(agentFile); err == nil {
		var aj map[string]interface{}
		if json.Unmarshal(data, &aj) == nil {
			if n, ok := aj["name"].(string); ok {
				agentName = n
			}
			if d, ok := aj["description"].(string); ok {
				agentDesc = d
			}
		}
	}

	// --- Domains (unique from artifacts) ---
	domainSet := map[string]bool{}
	for _, a := range idx.Artifacts {
		if a.Domain != "" {
			// Top-level domain only
			parts := strings.SplitN(a.Domain, "/", 2)
			domainSet[parts[0]] = true
		}
	}
	domains := make([]string, 0, len(domainSet))
	for d := range domainSet {
		domains = append(domains, d)
	}
	sort.Strings(domains)

	// --- Artifact types (from index) ---
	typeCount := map[string]int{}
	for _, a := range idx.Artifacts {
		if a.Type != "" {
			typeCount[a.Type]++
		}
	}
	types := make([]string, 0, len(typeCount))
	for t := range typeCount {
		// Skip regex fallback artifacts (invalid type names)
		if strings.ContainsAny(t, "<>|()") {
			continue
		}
		types = append(types, t)
	}
	sort.Strings(types)

	// --- Skills ---
	skillsDir := filepath.Join(root, ".claude", "skills")
	var skills []string
	if entries, err := os.ReadDir(skillsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				skillFile := filepath.Join(skillsDir, e.Name(), "SKILL.md")
				if _, err := os.Stat(skillFile); err == nil {
					skills = append(skills, e.Name())
				}
			}
		}
	}

	// --- Scripts ---
	scriptsDir := filepath.Join(root, "tools", "scripts")
	var scripts []string
	if entries, err := os.ReadDir(scriptsDir); err == nil {
		for _, e := range entries {
			name := e.Name()
			if strings.HasSuffix(name, ".py") || strings.HasSuffix(name, ".sh") {
				scripts = append(scripts, name)
			}
		}
	}

	// --- Dashboards ---
	uiDir := filepath.Join(root, "tools", "ui")
	var dashboards []string
	if entries, err := os.ReadDir(uiDir); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".html") {
				dashboards = append(dashboards, strings.TrimSuffix(e.Name(), ".html"))
			}
		}
	}

	// --- Deliverables ---
	totalDeliv, shippedDeliv, coveredNodes := scanDeliverableCoverage()
	totalNodes := countValueChainNodes()

	// Deliverable details
	type delivInfo struct {
		Name      string   `json:"name"`
		Readiness string   `json:"readiness"`
		Nodes     []string `json:"chain_nodes"`
	}
	var deliverables []delivInfo
	delivDir := filepath.Join(root, "out", "deliverables")
	if entries, err := os.ReadDir(delivDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			readmePath := filepath.Join(delivDir, e.Name(), "README.md")
			data, err := os.ReadFile(readmePath)
			if err != nil {
				continue
			}
			fm, _ := parseFrontmatter(string(data))
			nodes := fmStringList(fm, "chain_nodes")
			if len(nodes) == 0 {
				continue
			}
			readiness := fmString(fm, "readiness")
			if readiness == "" {
				readiness = "draft"
			}
			deliverables = append(deliverables, delivInfo{
				Name:      e.Name(),
				Readiness: readiness,
				Nodes:     nodes,
			})
		}
	}

	// --- Validation stats ---
	met := 0
	for _, a := range idx.Artifacts {
		if a.Tier >= requiredTierFor(a) {
			met++
		}
	}
	compliance := compliancePct(met, len(idx.Artifacts))

	// --- Epistemic distribution ---
	epistemicDist := map[string]int{"proven": 0, "validated": 0, "observed": 0, "anecdotal": 0, "unassessed": 0}
	for _, a := range idx.Artifacts {
		if a.Confidence != "" {
			epistemicDist[a.Confidence]++
		} else {
			epistemicDist["unassessed"]++
		}
	}

	// --- JSON output ---
	if mode == ModeJSON {
		out := map[string]interface{}{
			"agent":       agentName,
			"description": agentDesc,
			"domains":     domains,
			"artifact_slots": map[string]interface{}{
				"types":      types,
				"total":      len(idx.Artifacts),
				"compliance": compliance,
			},
			"tool_slots": map[string]interface{}{
				"skills":     skills,
				"scripts":    scripts,
				"dashboards": dashboards,
			},
			"delivery": map[string]interface{}{
				"deliverables":  deliverables,
				"total":         totalDeliv,
				"shipped":       shippedDeliv,
				"nodes_covered": coveredNodes,
				"nodes_total":   totalNodes,
			},
			"epistemic": epistemicDist,
		}
		jsonPrint(out)
		return
	}

	// --- Text output ---
	fmt.Printf("\n  Agent: %s\n", agentName)
	if agentDesc != "" {
		fmt.Printf("  %s\n", agentDesc)
	}
	fmt.Printf("  Domains: %s\n", strings.Join(domains, ", "))

	fmt.Printf("\n  Artifact slots: %d types, %d artifacts, %d%% compliance\n",
		len(types), len(idx.Artifacts), compliance)
	// Show types in compact columns
	line := "    "
	for i, t := range types {
		entry := fmt.Sprintf("%-16s", t)
		line += entry
		if (i+1)%4 == 0 && i+1 < len(types) {
			fmt.Println(line)
			line = "    "
		}
	}
	if strings.TrimSpace(line) != "" {
		fmt.Println(line)
	}

	fmt.Printf("\n  Tool slots:\n")
	fmt.Printf("    Skills:     %d  (.claude/skills/)\n", len(skills))
	fmt.Printf("    Scripts:    %d  (tools/scripts/)\n", len(scripts))
	fmt.Printf("    Dashboards: %d  (tools/ui/)\n", len(dashboards))

	fmt.Printf("\n  Delivery: %d deliverables (%d shipped), %d/%d nodes covered\n",
		totalDeliv, shippedDeliv, coveredNodes, totalNodes)

	// Readiness breakdown
	byReadiness := map[string]int{}
	for _, d := range deliverables {
		byReadiness[d.Readiness]++
	}
	if len(byReadiness) > 0 {
		parts := []string{}
		for _, r := range []string{"shipped", "packaged", "draft"} {
			if c, ok := byReadiness[r]; ok {
				parts = append(parts, fmt.Sprintf("%d %s", c, r))
			}
		}
		fmt.Printf("    Readiness:  %s\n", strings.Join(parts, " · "))
	}

	fmt.Printf("\n  Epistemic coverage:\n")
	for _, level := range []string{"proven", "validated", "observed", "anecdotal"} {
		c := epistemicDist[level]
		if c > 0 {
			fmt.Printf("    %-12s %3d\n", level, c)
		}
	}
	fmt.Printf("    %-12s %3d\n", "unassessed", epistemicDist["unassessed"])

	fmt.Printf("\n  Validation:\n")
	fmt.Printf("    ./ask lint   %d artifacts indexed\n", len(idx.Artifacts))
	fmt.Printf("    ./ask audit  %d%% compliance\n", compliance)

	fmt.Println()
}
