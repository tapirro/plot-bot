package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func cmdLint(args []string, mode OutputMode, idx *Index) error {
	items := idx.Artifacts
	var fix, gap bool
	var zoneFilter, typeFilter string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--fix":
			fix = true
		case "--gap":
			gap = true
		case "--zone":
			i++
			if i < len(args) {
				zoneFilter = args[i]
			}
		case "--type":
			i++
			if i < len(args) {
				typeFilter = args[i]
			}
		}
	}

	if zoneFilter != "" {
		items = filterArtifacts(items, func(a Artifact) bool {
			return strings.HasPrefix(a.Path, zoneFilter)
		})
	}

	if typeFilter != "" {
		types := map[string]bool{}
		for _, t := range strings.Split(typeFilter, ",") {
			types[strings.TrimSpace(t)] = true
		}
		items = filterArtifacts(items, func(a Artifact) bool {
			return types[a.Type]
		})
	}

	var errors, warnings []string
	tierCounts := [4]int{}
	typeTiers := map[string][4]int{}

	for _, a := range items {
		t := a.Tier
		if t > 3 {
			t = 3
		}
		tierCounts[t]++
		tc := typeTiers[a.Type]
		tc[t]++
		typeTiers[a.Type] = tc
	}

	idMap := map[string][]string{}
	for _, a := range items {
		isIntake := strings.HasPrefix(a.Path, "in/")
		isTemplate := a.Type == "template" || strings.HasPrefix(a.Path, "tools/templates/")

		if !isIntake {
			idMap[a.ID] = append(idMap[a.ID], a.Path)
		}

		if !knownTypes[a.Type] && !isTemplate {
			errors = append(errors, fmt.Sprintf("ERR  %s  unknown type: %s", a.Path, a.Type))
		}

		if !isIntake && !isTemplate && validStatuses[a.Type] != nil {
			valid := false
			for _, s := range validStatuses[a.Type] {
				if strings.EqualFold(s, a.Status) {
					valid = true
					break
				}
			}
			if !valid {
				errors = append(errors, fmt.Sprintf("ERR  %s  invalid status '%s' for type '%s'", a.Path, a.Status, a.Type))
			}
		}

		if a.Tier == 0 && !isIntake && !strings.HasPrefix(a.Path, "tools/scripts/") && !isTemplate {
			warnings = append(warnings, fmt.Sprintf("WARN %s  no frontmatter (tier 0)", a.Path))
		}

		// Confidence validation
		if a.Confidence != "" && !validConfidence[a.Confidence] {
			errors = append(errors, fmt.Sprintf("ERR  %s  invalid confidence '%s' (valid: anecdotal, observed, validated, proven)", a.Path, a.Confidence))
		}
		if confidenceRecommended[a.Type] && a.Confidence == "" && a.Tier >= 2 {
			warnings = append(warnings, fmt.Sprintf("WARN %s  type '%s' at tier %d without confidence", a.Path, a.Type, a.Tier))
		}

		for _, e := range a.EdgesOut {
			if _, err := os.Stat(filepath.Join(root, e)); err != nil {
				found := false
				for _, item := range idx.Artifacts {
					if item.Path == e {
						found = true
						break
					}
				}
				if !found {
					warnings = append(warnings, fmt.Sprintf("WARN %s  broken link: %s", a.Path, e))
				}
			}
		}
	}

	for id, paths := range idMap {
		if len(paths) > 1 {
			hasFMID := false
			for _, p := range paths {
				for _, a := range items {
					if a.Path == p && a.Tier >= 1 {
						hasFMID = true
						break
					}
				}
			}
			if hasFMID {
				errors = append(errors, fmt.Sprintf("ERR  duplicate id '%s': %s", id, strings.Join(paths, ", ")))
			} else {
				warnings = append(warnings, fmt.Sprintf("WARN duplicate auto-id '%s': %s", id, strings.Join(paths, ", ")))
			}
		}
	}

	if fix {
		fixed := 0
		for _, a := range items {
			if a.Tier == 0 && strings.HasSuffix(a.Path, ".md") &&
				!strings.HasPrefix(a.Path, "in/") && !strings.HasPrefix(a.Path, "tools/scripts/") {
				fpath := filepath.Join(root, a.Path)
				content, err := os.ReadFile(fpath)
				if err != nil {
					continue
				}
				if !strings.HasPrefix(string(content), "---") {
					fm := fmt.Sprintf("---\nid: %s\ntype: %s\nstatus: %s\n---\n\n", a.ID, a.Type, a.Status)
					os.WriteFile(fpath, []byte(fm+string(content)), 0644)
					fixed++
				}
			}
		}
		if mode != ModeJSON {
			fmt.Printf("Fixed: %d files\n", fixed)
		}
	}

	// Compliance
	type typeCompliance struct {
		Total    int `json:"total"`
		Meet     int `json:"meet"`
		Gap      int `json:"gap"`
		Required int `json:"required_tier"`
	}
	compliance := map[string]*typeCompliance{}
	var gapFiles []Artifact
	totalMeet, totalGap := 0, 0

	for _, a := range items {
		if strings.Contains(a.Path, "templates/") {
			continue
		}
		req := requiredTierFor(a)
		tc, ok := compliance[a.Type]
		if !ok {
			tc = &typeCompliance{Required: req}
			compliance[a.Type] = tc
		}
		tc.Total++
		if a.Tier >= req {
			tc.Meet++
			totalMeet++
		} else {
			tc.Gap++
			totalGap++
			gapFiles = append(gapFiles, a)
		}
	}

	compPct := compliancePct(totalMeet, len(items))

	worstGapType := ""
	worstGapPct := 101
	for tn, tc := range compliance {
		if tc.Gap > 0 && !strings.HasPrefix(tn, "<") {
			pct := compliancePct(tc.Meet, tc.Total)
			if pct < worstGapPct {
				worstGapPct = pct
				worstGapType = tn
			}
		}
	}

	// Output
	if mode == ModeJSON {
		result := map[string]interface{}{
			"tiers": map[string]interface{}{
				"t0": tierCounts[0], "t1": tierCounts[1],
				"t2": tierCounts[2], "t3": tierCounts[3],
			},
			"by_type":        typeTiers,
			"compliance":     compliance,
			"compliance_pct": compPct,
			"errors":         len(errors),
			"warnings":       len(warnings),
			"details":        append(errors, warnings...),
			"pass":           len(errors) == 0,
		}
		if gap {
			var gapList []map[string]interface{}
			for _, a := range gapFiles {
				_, missing := tierDiag(a)
				gapList = append(gapList, map[string]interface{}{
					"id": a.ID, "path": a.Path, "type": a.Type,
					"tier": a.Tier, "required": requiredTierFor(a),
					"missing": missing,
				})
			}
			result["gap_files"] = gapList
		}
		jsonPrintCompact(result)
	} else if gap {
		fmt.Printf("COMPLIANCE: %d/%d (%d%%)\n\n", totalMeet, len(items), compPct)
		fmt.Printf("%-20s %5s %4s %4s %4s  %s\n", "TYPE", "TOTAL", "MEET", "GAP", "REQ", "COMPLIANCE")

		typeNames := make([]string, 0, len(compliance))
		for tn := range compliance {
			if strings.HasPrefix(tn, "<") {
				continue
			}
			typeNames = append(typeNames, tn)
		}
		sort.Strings(typeNames)

		for _, tn := range typeNames {
			tc := compliance[tn]
			pct := compliancePct(tc.Meet, tc.Total)
			marker := ""
			if tc.Gap > 0 {
				marker = " <<"
			}
			fmt.Printf("%-20s %5d %4d %4d   T%d  %3d%%%s\n", tn, tc.Total, tc.Meet, tc.Gap, tc.Required, pct, marker)
		}

		if len(gapFiles) > 0 && mode == ModeWide {
			fmt.Printf("\nGAP FILES (%d):\n", len(gapFiles))
			for _, a := range gapFiles {
				_, missing := tierDiag(a)
				missingStr := ""
				if len(missing) > 0 {
					missingStr = " [add: " + strings.Join(missing, ", ") + "]"
				}
				fmt.Printf("  T%d→T%d  %-12s %s%s\n", a.Tier, requiredTierFor(a), a.Type, a.Path, missingStr)
			}
		} else if len(gapFiles) > 0 {
			fmt.Printf("\n%d files below required tier\n", len(gapFiles))
		}

		if mode != ModeJSON && worstGapType != "" {
			if totalGap > 0 && mode != ModeWide {
				fmt.Printf("hint: worst: %s(%d%%). try: ./ask lint --gap --type %s -w\n", worstGapType, worstGapPct, worstGapType)
			}
		}
	} else if mode == ModeWide {
		fmt.Println("TIER COVERAGE")
		total := len(items)
		labels := []string{"tier 0 (file only)", "tier 1 (id+tags)", "tier 2 (enriched)", "tier 3 (sections)"}
		for i, label := range labels {
			pct := compliancePct(tierCounts[i], total)
			fmt.Printf("  %-22s %4d  %3d%%\n", label, tierCounts[i], pct)
		}
		fmt.Printf("  %-22s %4d  100%%\n", "TOTAL", total)
		fmt.Printf("\nCOMPLIANCE: %d/%d (%d%%)\n", totalMeet, len(items), compPct)
		fmt.Println()

		fmt.Printf("%-20s %5s %4s %4s %4s %4s\n", "BY TYPE", "TOTAL", "T0", "T1", "T2", "T3")
		typeNames := sortMapByKey2(typeTiers)
		for _, tn := range typeNames {
			tc := typeTiers[tn]
			total := tc[0] + tc[1] + tc[2] + tc[3]
			fmt.Printf("%-20s %5d %4d %4d %4d %4d\n", tn, total, tc[0], tc[1], tc[2], tc[3])
		}
		fmt.Println()

		for _, e := range errors {
			fmt.Println(e)
		}
		for _, w := range warnings {
			fmt.Println(w)
		}
		status := "PASS"
		if len(errors) > 0 {
			status = "FAIL"
		}
		fmt.Printf("\nRESULT: %s (%d errors, %d warnings)\n", status, len(errors), len(warnings))

		if totalGap > 0 {
			fmt.Printf("hint: %d gaps remain. worst: %s(%d%%). try: ./ask lint --gap --type %s -w\n", totalGap, worstGapType, worstGapPct, worstGapType)
		}
	} else {
		fmt.Printf("tiers: t0:%d t1:%d t2:%d t3:%d total:%d compliance:%d%%\n",
			tierCounts[0], tierCounts[1], tierCounts[2], tierCounts[3], len(items), compPct)
		if len(errors) > 0 {
			for _, e := range errors {
				fmt.Println(e)
			}
		}
		fmt.Printf("%d errors, %d warnings\n", len(errors), len(warnings))
		if totalGap > 0 {
			fmt.Printf("hint: %d below target. worst: %s(%d%%). try: ./ask lint --gap\n", totalGap, worstGapType, worstGapPct)
		}
		if len(errors) > 0 {
			return fmt.Errorf("lint: %d errors found", len(errors))
		}
	}
	return nil
}
