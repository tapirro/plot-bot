package main

import (
	"fmt"
	"strings"
)

// --- Domain tree ---

type domainNode struct {
	Name     string       `json:"name"`
	Total    int          `json:"total"`
	Direct   int          `json:"direct"`
	Children []domainNode `json:"children,omitempty"`
}

var domainTree = []struct {
	parent   string
	children []string
}{
	{"hilart", []string{"resale", "ops", "cpa", "dev", "dwh"}},
	{"voic", []string{"research", "app", "bot-seller"}},
	{"hive", []string{"infra", "network"}},
	{"assistant", []string{"awr", "analytics", "design", "tooling"}},
	{"corp-dev", []string{"bible", "governance", "strategy"}},
	{"real-estate", []string{"batumi"}},
	{"cross-agent", []string{"patterns", "packages", "methodologies"}},
	{"distillat", nil},
}

func cmdDomainTree(args []string, mode OutputMode, idx *Index) {
	// Count artifacts by domain
	byDomain := map[string]int{}
	untagged := 0
	for _, a := range idx.Artifacts {
		if a.Domain != "" {
			byDomain[a.Domain]++
		} else {
			untagged++
		}
	}

	// Build tree
	var nodes []domainNode
	for _, dt := range domainTree {
		node := domainNode{Name: dt.parent, Direct: byDomain[dt.parent]}
		for _, c := range dt.children {
			full := dt.parent + "/" + c
			cn := domainNode{Name: c, Total: byDomain[full], Direct: byDomain[full]}
			node.Children = append(node.Children, cn)
			node.Total += byDomain[full]
		}
		node.Total += node.Direct
		nodes = append(nodes, node)
	}

	if mode == ModeJSON {
		result := map[string]interface{}{}
		for _, n := range nodes {
			children := map[string]int{}
			for _, c := range n.Children {
				children[c.Name] = c.Total
			}
			result[n.Name] = map[string]interface{}{
				"total": n.Total, "direct": n.Direct, "children": children,
			}
		}
		result["mantissa"] = map[string]interface{}{"total": byDomain["mantissa"]}
		result["_untagged"] = untagged
		result["_total"] = len(idx.Artifacts)
		jsonPrintCompact(result)
		return
	}

	// Drill down into a specific domain?
	filterDomain := ""
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			filterDomain = a
			break
		}
	}

	fmt.Printf("Domain Tree (%d artifacts, %d untagged)\n\n", len(idx.Artifacts), untagged)

	for _, n := range nodes {
		if filterDomain != "" && !strings.HasPrefix(n.Name, filterDomain) && !strings.HasPrefix(filterDomain, n.Name) {
			continue
		}
		bar := barStr(n.Total, 20)
		fmt.Printf("  %-20s %3d  %s\n", n.Name, n.Total, bar)
		for _, c := range n.Children {
			if c.Total > 0 {
				cbar := barStr(c.Total, 15)
				fmt.Printf("    %-18s %3d  %s\n", c.Name, c.Total, cbar)
			} else {
				fmt.Printf("    %-18s   -\n", c.Name)
			}
		}
		if n.Direct > 0 && len(n.Children) > 0 {
			fmt.Printf("    (direct):          %3d\n", n.Direct)
		}
	}

	if m := byDomain["mantissa"]; m > 0 {
		fmt.Printf("\n  mantissa (root)     %3d  (raw containers)\n", m)
	}
	if untagged > 0 {
		fmt.Printf("  (untagged)          %3d\n", untagged)
	}
}

