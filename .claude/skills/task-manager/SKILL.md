---
name: task-manager
description: "Task Manager — AWR Bet & Action Lifecycle"
argument-hint: "[bet-id action-index | list | done | assign]"
allowed-tools: Bash, Read, Grep
---

# Task Manager — AWR Bet & Action Lifecycle

Manage bets, actions, and deliverables through their lifecycle using `./ask` commands. This skill defines the canonical workflow for creating, tracking, and completing work items.

## Pre-computed State
### Open Actions
!`./ask @todo -q 2>/dev/null`

### Progress by Domain
!`./ask @progress -q 2>/dev/null`

### Active Bets
!`./ask @bets -q 2>/dev/null`

## When to use

- Creating a new bet (initiative/project) from a decision or research finding
- Breaking a bet into actionable steps
- Updating progress (marking actions done, changing bet status)
- Reviewing open work (`@todo`, `@actions`)
- Packaging completed work into deliverables

## Core Concepts

### Bet = Initiative with Hypothesis

A bet is a project with a testable hypothesis and measurable outcomes. Bets live in `work/bets/<slug>.md`.

### Action = Concrete Step inside a Bet

Actions are checkboxes in the `## Actions` section of a bet. Each has an owner and optional priority weight.

### Deliverable = Packaged Output

A deliverable is a self-contained folder in `out/deliverables/` with README, artifacts, and documentation.

## Bet Lifecycle

```
proposed → experiment → validated → scaled
                ↓
             failed (with learnings)
```

| Status | Meaning | Who can transition |
|--------|---------|-------------------|
| `proposed` | Idea captured, not yet approved | Anyone creates, Vadim approves |
| `experiment` | Approved, work in progress | Vadim approves → agent executes |
| `validated` | Hypothesis confirmed by metrics | Agent proposes, Vadim confirms |
| `scaled` | Rolled out to production | Vadim decides |
| `failed` | Hypothesis disproven | Agent proposes, Vadim confirms |

## Quick Reference — `./ask` Commands

### Discovery & Status

```bash
# See all open work
./ask @todo                          # All open actions across bets
./ask @actions                       # Same, more verbose
./ask actions -o assistant           # My actions only
./ask actions -s todo                # Only unchecked
./ask actions --blocked              # Blocked by dependencies

# Health check
./ask h                              # Full workshop health
./ask l -t bet -q                    # List all bets
./ask l -t bet --where "status=experiment" -q  # Active bets only

# Inspect a bet
./ask g bet-resale-intelligence      # Full content
./ask g bet-resale-intelligence#actions  # Actions only
./ask g bet-resale-intelligence#metrics  # Metrics only
./ask b bet-resale-intelligence      # Block TOC (structure overview)
```

### Progress Tracking

```bash
# Mark an action as done
./ask done bet-resale-intelligence 5   # Toggle action #5 (1-indexed from Actions section)

# Assign an action to an owner
./ask assign bet-resale-intelligence 3 vadim  # Set @vadim on action #3

# Change artifact status
./ask status bet-resale-intelligence active   # Change status in frontmatter

# Domain-level progress overview
./ask @progress                        # done/total/blocked per domain
./ask progress -j                      # Same, JSON output

# After changes, verify
./ask actions -p bet-resale-intelligence  # Show updated state
```

### Creating Bets

1. Write the bet file: `work/bets/<slug>.md`
2. Use template from `out/deliverables/artifact-system/examples/bet_example_template.md`
3. Required frontmatter: `id`, `type: bet`, `status`, `owner`, `tags`
4. Required sections: `## Problem`, `## Solution`, `## Metrics`, `## Actions`, `## Next Experiment`
5. Scan: `./ask scan -q`
6. Verify: `./ask g bet-<slug> -m`

### Creating Deliverables

1. Create folder: `out/deliverables/<name>/`
2. Write `README.md` with frontmatter: `id`, `type: guide`, `status`, `tags`, `parent` (bet ID)
3. Status: `draft` → `ready` → `delivered`
4. Scan: `./ask scan -q`

## Rules for Agent (MANDATORY)

### When to Create a Bet
- Vadim says "давай сделаем X" or approves an initiative
- Research yields actionable findings (like resale transcript analysis → 3 deliverables)
- A recurring problem needs structured solution

### When NOT to Create a Bet
- One-off tasks (just do them)
- Explorations without clear hypothesis (use `work/topics/` instead)
- External requests without Vadim's approval

### Action Formatting

```markdown
## Actions

- [x] Completed action description @owner
- [ ] Open action description @owner (priority_weight)
- [!] Blocked action description @owner ⛔ blocking-bet-id
```

- `@owner`: `assistant`, `vadim`, `dima`, or specific person
- `(priority_weight)`: optional float, higher = more important. Used by `./ask actions` for sorting
- `[!]` + `⛔`: blocked by another bet/dependency
- Actions are **1-indexed** for `./ask done` command

### Progress Protocol

1. **Starting work on a bet action:** Read the bet first (`./ask g <bet-id>#actions`), then work
2. **After completing an action:** `./ask done <bet-id> <index>` — immediately, don't batch
3. **After completing a deliverable group (D1/D2/D3):** Update bet status if all actions in group are done
4. **Weekly:** `./ask @todo` to review open work
5. **At session start:** Check `./ask actions -o assistant -s todo` for pending work

### Status Transitions

| From → To | Trigger | Who |
|-----------|---------|-----|
| proposed → experiment | Vadim says "go" / "одобряю" / "делай" | Vadim decides, agent updates frontmatter |
| experiment → validated | Metrics met (check `## Metrics` section) | Agent proposes with evidence, Vadim confirms |
| experiment → failed | Metrics not met after full execution | Agent proposes with evidence, Vadim confirms |
| validated → scaled | Production rollout decision | Vadim decides |

To update status: `./ask status <bet-id> <new-status>`, then `./ask scan -q`.

### Deliverable Packaging Checklist

Before marking a deliverable as `status: ready`:

- [ ] README.md with clear description and contents table
- [ ] All files listed in README actually exist
- [ ] Self-contained: no broken references to external files
- [ ] Format matches audience (HTML for operators, XLSX for managers, PDF for print)
- [ ] Tested: dashboards load, scripts run, files open correctly

## Integration with Hive Tasks

If the bet has corresponding Hive tasks (`hive-tasks.sh`), sync status:
```bash
./tools/scripts/hive-complete-task.sh <task-id>  # When completing a Hive task
```

Hive tasks are separate from AWR bet actions — they may overlap but are not 1:1.

## Examples

### Create bet from research findings
```bash
# 1. Write bet file
# 2. Scan
./ask scan -q
# 3. Verify
./ask g bet-new-bet -m
./ask actions -p bet-new-bet
```

### Complete a sequence of actions
```bash
# Work on action 5...
# [do the work]
./ask done bet-resale-intelligence 5
# Work on action 6...
# [do the work]
./ask done bet-resale-intelligence 6
# Verify progress
./ask actions -p bet-resale-intelligence
```

### Weekly review
```bash
./ask @todo                    # All open work
./ask l -t bet --where "status=experiment" -q  # Active bets
./ask actions -o assistant --blocked   # What's stuck
```
