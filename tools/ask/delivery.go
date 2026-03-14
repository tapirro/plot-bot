package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// scanDeliverableCoverage scans out/deliverables/*/README.md for chain_nodes frontmatter.
// Returns (totalDeliverables, shippedCount, uniqueNodesCovered).
func scanDeliverableCoverage() (int, int, int) {
	delivDir := filepath.Join(root, "out", "deliverables")
	entries, err := os.ReadDir(delivDir)
	if err != nil {
		return 0, 0, 0
	}

	total := 0
	shipped := 0
	nodeSet := map[string]bool{}

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
		for _, n := range nodes {
			nodeSet[n] = true
		}
		total++
		if fmString(fm, "readiness") == "shipped" {
			shipped++
		}
	}

	return total, shipped, len(nodeSet)
}

// countValueChainNodes counts node IDs in YAML files by looking for "  - id:" lines.
func countValueChainNodes() int {
	chainsDir := filepath.Join(root, "knowledge", "value_chains")
	entries, err := os.ReadDir(chainsDir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(chainsDir, e.Name()))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "- id:") || strings.HasPrefix(trimmed, "id:") {
				// Only count node IDs (not chain IDs which start with "chain-")
				val := strings.TrimSpace(strings.TrimPrefix(trimmed, "- id:"))
				val = strings.TrimSpace(strings.TrimPrefix(val, "id:"))
				if !strings.HasPrefix(val, "chain-") {
					count++
				}
			}
		}
	}
	return count
}

// cmdDelivery: deliverable -> chain node coverage via vchain.py
func cmdDelivery(args []string, mode OutputMode, idx *Index) {
	vchainPath := filepath.Join(root, "tools", "scripts", "vchain.py")
	cmdArgs := []string{vchainPath, "deliverables"}

	// Pass through flags
	for _, a := range args {
		switch a {
		case "--unmapped":
			cmdArgs = append(cmdArgs, "--unmapped")
		case "--json", "-j":
			cmdArgs = append(cmdArgs, "--json")
		}
	}

	if mode == ModeJSON {
		hasJSON := false
		for _, a := range cmdArgs {
			if a == "--json" {
				hasJSON = true
				break
			}
		}
		if !hasJSON {
			cmdArgs = append(cmdArgs, "--json")
		}
	}

	cmd := exec.Command("python3", cmdArgs...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "vchain.py error: %v\n", err)
		return
	}
	fmt.Print(string(out))
}

// cmdDeliveries: show delivery log via vchain.py
func cmdDeliveries(args []string, mode OutputMode, idx *Index) {
	vchainPath := filepath.Join(root, "tools", "scripts", "vchain.py")
	cmdArgs := []string{vchainPath, "log"}

	hasPending := false
	for _, a := range args {
		switch a {
		case "--pending":
			cmdArgs = append(cmdArgs, "--pending")
			hasPending = true
		}
	}

	if mode == ModeJSON {
		cmdArgs = append(cmdArgs, "--json")
	}

	// Default: show pending if no flags
	if !hasPending && mode != ModeJSON {
		cmdArgs = append(cmdArgs, "--pending")
	}

	cmd := exec.Command("python3", cmdArgs...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		// No log yet — show hint
		if mode == ModeJSON {
			fmt.Println(`{"entries":[],"total":0}`)
		} else {
			fmt.Println("0 deliveries. Use: vchain.py deliver <name> <receiver>")
		}
		return
	}
	fmt.Print(string(out))
}
