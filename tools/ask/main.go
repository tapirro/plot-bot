package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var root string

func findRoot() string {
	exe, err := os.Executable()
	if err != nil {
		wd, _ := os.Getwd()
		return wd
	}
	dir := filepath.Dir(exe)
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err == nil {
		return dir
	}
	wd, _ := os.Getwd()
	return wd
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		switch err.(type) {
		case *UsageError:
			os.Exit(2)
		default:
			os.Exit(1)
		}
	}
}

func run() error {
	initRules()
	initTypes()
	initDomainRules()
	root = findRoot()

	args := os.Args[1:]
	if len(args) == 0 {
		printUsage()
		return nil
	}

	mode := ModeAgent
	var cleaned []string
	for _, arg := range args {
		switch arg {
		case "-j":
			mode = ModeJSON
		case "-w":
			mode = ModeWide
		case "-q":
			quietMode = true
		case "--git-only":
			scanGitOnly = true
		default:
			cleaned = append(cleaned, arg)
		}
	}
	args = cleaned

	if len(args) == 0 {
		printUsage()
		return nil
	}

	cmd := args[0]
	cmdArgs := args[1:]

	// Views: @name
	if strings.HasPrefix(cmd, "@") {
		name := cmd[1:]
		if name == "" {
			listViews(mode)
			return nil
		}
		if !runView(name, cmdArgs, mode) {
			fmt.Fprintf(os.Stderr, "unknown view: @%s\n", name)
			listViews(mode)
			return &UsageError{Msg: fmt.Sprintf("unknown view: @%s", name)}
		}
		return nil
	}

	switch cmd {
	case "list", "l":
		idx := loadIndex()
		cmdList(cmdArgs, mode, idx)
	case "get", "g":
		idx := loadIndex()
		return cmdGet(cmdArgs, mode, idx)
	case "find", "f":
		idx := loadIndex()
		cmdFind(cmdArgs, mode, idx)
	case "sum", "s":
		idx := loadIndex()
		cmdSum(cmdArgs, mode, idx)
	case "map", "m":
		idx := loadIndex()
		cmdMap(cmdArgs, mode, idx)
	case "lint":
		idx := loadIndex()
		return cmdLint(cmdArgs, mode, idx)
	case "blocks", "b":
		idx := loadIndex()
		return cmdBlocks(cmdArgs, mode, idx)
	case "tag":
		idx := loadIndex()
		cmdTag(cmdArgs, mode, idx)
	case "health", "h":
		cmdHealth(cmdArgs, mode)
	case "digest":
		idx := loadIndex()
		cmdDigest(cmdArgs, mode, idx)
	case "scan":
		return cmdScan(cmdArgs, mode)
	case "rate":
		fmt.Fprintln(os.Stderr, "removed: use node ~/.claude/rate-control/claude_limits.mjs")
		return &UsageError{Msg: "rate command removed — use claude_limits.mjs"}
	case "audit", "a":
		idx := loadIndex()
		return cmdAudit(cmdArgs, mode, idx)
	case "fix":
		idx := loadIndex()
		cmdFix(cmdArgs, mode, idx)
	case "actions":
		idx := loadIndex()
		return cmdActions(cmdArgs, mode, idx)
	case "done":
		idx := loadIndex()
		return cmdDone(cmdArgs, mode, idx)
	case "assign":
		idx := loadIndex()
		return cmdAssign(cmdArgs, mode, idx)
	case "status":
		idx := loadIndex()
		return cmdStatus(cmdArgs, mode, idx)
	case "progress":
		idx := loadIndex()
		cmdProgress(cmdArgs, mode, idx)
	case "batch":
		return cmdBatch(cmdArgs, mode)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		printUsage()
		return &UsageError{Msg: fmt.Sprintf("unknown command: %s", cmd)}
	}
	return nil
}

func printUsage() {
	fmt.Println(`ask — AWR artifact navigator

usage: ./ask <command> [args] [-j] [-w] [-q]

commands (short aliases in parens):
  list (l)    query/filter/search artifacts
  get  (g)    read artifact, section, or block
  find (f)    fuzzy search + instant context
  blocks (b)  TOC with block sizes, roles, schema
  health (h)  workshop health: compliance, gaps, stale, errors
  digest      extract role blocks from insights
  sum  (s)    aggregate counters
  map  (m)    reference graph
  lint        validate frontmatter + links
  tag         tag frequency table
  audit (a)   block contracts audit + fix plan
  fix         pre-extract context + deterministic fixes
  scan        rebuild index cache
  rate        (removed → claude_limits.mjs)
  actions     list/filter inline actions from bets+maintenance
  done        toggle action done: ask done <parent-id> <index>
  assign      set action owner: ask assign <parent-id> <index> <owner>
  status      change artifact status: ask status <id> <new-status>
  progress    action progress by domain (done/total/blocked)
  batch       multiple commands in one call
  @<view>     named view (@health @stale @recent @tier0 @orphans @patterns @decisions @audit @actions @todo @progress)

flags:
  -j     JSON    -w  wide    -q  quiet (no hints)

list flags:
  -t T          --type T              -f f1,f2   --fields f1,f2
  -S F          --sort F              -l N       --limit N
  -c            --count               -g F       --group-by F
  --tag T       --status S            --zone Z   --tier N
  --where "..."                       --past MMDD / --old / --gap
  --content-match "regex"             --has-section "slug"

get flags:
  -m   --meta     -d  --diag    --only f1,f2
  id#role                               single block by role/slug
  id#role1,role2                        multi-block extraction
  id1,id2,id3                           multi-get

blocks flags:
  -s   --schema                         validate vs type schema
  -t T --role R                         cross-search role R in type T

audit flags:
  -t T  --type T     filter by artifact type
  -k K  --kind K     filter by violation kind (stub/bloated/missing/hollow/format)
  --fix              show only fix plan

fix flags:
  --plan             pre-extract context as JSON
  --stub             deterministic stub expansion (no LLM)
  --dispatch         generate batched subagent prompts
  --batch-size N     tasks per batch (default: 8)
  --dry-run / -n     preview without changes
  -t T  --type T     filter by type
  -k K  --kind K     filter by kind

actions flags:
  -p ID  --parent ID   filter by parent artifact
  -o X   --owner X     filter by @owner
  -s S   --status S    done | todo | blocked
  --blocked            show only blocked actions`)

}

// cmdBatch runs multiple ask commands in one call, collecting results.
func cmdBatch(args []string, mode OutputMode) error {
	if len(args) == 0 {
		return &UsageError{Msg: "usage: ask batch \"cmd1 args\" \"cmd2 args\" ..."}
	}

	exe, _ := os.Executable()

	if mode == ModeJSON {
		var results []map[string]interface{}
		for _, cmdStr := range args {
			parts := strings.Fields(cmdStr)
			if len(parts) == 0 {
				continue
			}
			cmdArgs := append(parts, "-j")
			cmd := exec.Command(exe, cmdArgs...)
			cmd.Dir = root
			out, err := cmd.Output()
			result := map[string]interface{}{"command": cmdStr}
			if err != nil {
				result["error"] = err.Error()
			} else {
				var parsed interface{}
				if json.Unmarshal(out, &parsed) == nil {
					result["output"] = parsed
				} else {
					result["output"] = strings.TrimSpace(string(out))
				}
			}
			results = append(results, result)
		}
		data, _ := json.MarshalIndent(map[string]interface{}{"results": results}, "", "  ")
		fmt.Println(string(data))
	} else {
		for i, cmdStr := range args {
			if i > 0 {
				fmt.Println("---")
			}
			fmt.Printf("=== %s ===\n", cmdStr)
			parts := strings.Fields(cmdStr)
			if len(parts) == 0 {
				continue
			}
			cmd := exec.Command(exe, parts...)
			cmd.Dir = root
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Run()
		}
	}
	return nil
}
