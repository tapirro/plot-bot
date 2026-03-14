---
id: pattern-dispatch-fix
type: pattern
status: active
domain: assistant/tooling
tags: [agent-tooling, token-economics, quality, dispatch]
origin: self
confidence: validated
basis: "applied in 3 audit-fix dispatch cycles, 214→25 violations"
---

# Dispatch Fix — Parallel Quality Repair via Pre-Extracted Context

## Problem

Quality repair of AWR artifacts requires reading each file, understanding its structure, and making targeted edits. Traditional approach: launch N subagents with generic instructions → each agent reads files independently → duplicate reads, bridge turns, context bloat. Empirical data: 4 subagents × 80 reads × 12 bridge turns = 257K tokens, 240s.

Without pre-extraction, each subagent wastes 64-74% of its budget on cache creation (reading files for the first time). Bridge turns (text-only turns between tool calls) consume another 20-26%. Actual useful work (generating edits) = only 6-10% of cost.

## Solution

Pre-extract all violation context in Go (46ms), generate batched subagent prompts with embedded context, dispatch N agents in parallel. Each agent receives exact paths, violations, and source material — zero discovery reads needed. Measured: 2 agents × 12 reads × 12 edits = 124K tokens, 133s. **2x token savings, 4.6x fewer tool calls.**

The key insight: context extraction is a mechanical operation (Go script, no LLM needed), while content generation is semantic (requires LLM). Separating these two phases eliminates waste: Go handles the mechanical part in 46ms, LLM agents focus solely on generating quality content.

Cost model validated empirically: $0.74 per violation fixed, 50% edit efficiency (edits / total tool calls). Remaining inefficiency is bridge turns between read→edit pairs — further reducible by embedding file content directly in prompts (eliminating reads entirely).

## Workflow

```
1. SNAPSHOT     ./ask audit -j -q > /tmp/audit_before.json
2. DISPATCH     ./ask fix --dispatch [-k missing] [--batch-size 4]
3. LAUNCH       Copy batch prompts → Agent tool (parallel)
4. VERIFY       ./ask scan -q && ./ask audit -j -q > /tmp/audit_after.json
5. REPORT       python3 tools/scripts/dispatch_report.py <task_outputs> \
                  --audit-before /tmp/audit_before.json \
                  --audit-after /tmp/audit_after.json
```

## Batch Strategy

| Kind | Risk | Batch size | Strategy |
|------|------|-----------|----------|
| `missing` | Low | 4 | Template-based adds, high parallelism |
| `stub` | Low | 4 | Expand with source material |
| `format` | Low | 8 | Restructure only, preserve content |
| `bloated` | High | 2 | Compress while preserving meaning, needs review |

Process kinds in order: missing → stub → format → bloated. Re-run audit between kinds.

## Prompt Rules

Mandatory elements in every dispatch prompt:

1. **Role instruction** — one line: "Fix AWR artifacts by [action]. Use Edit tool only."
2. **Processing order** — "Process tasks sequentially: read file → edit → next file."
3. **No filler** — "Do NOT add TODO/TBD. Do NOT explain. Just fix."
4. **Language** — "Use same language as existing content."
5. **Min-lines hint** — "## Decisions must have ≥1 content line. ## Problem ≥3 lines. ## Solution ≥5 lines."
6. **Idempotency guard** — "If ## [Section] already exists, SKIP — do not duplicate."
7. **Insertion point** — "Insert before last section or at end of file."

## Cost Model (Opus, empirical)

| Component | % of cost | Driver |
|-----------|-----------|--------|
| Cache creation | 64-74% | First read of each file |
| Cache read | 20-26% | Subsequent reads (prompt caching) |
| Output tokens | 6-10% | Generated content |
| Input tokens | <1% | Negligible with caching |

**Key insight:** Cache creation dominates. Minimizing unique file reads is the primary optimization lever. Pre-extracted context reduces reads from ~80 to ~12 (1 per file).

## Measurement

Single command for full report:

```bash
python3 tools/scripts/dispatch_report.py <task1.output> <task2.output> \
  --audit-before /tmp/audit_before.json \
  --audit-after /tmp/audit_after.json
```

Or auto-discover recent tasks:

```bash
python3 tools/scripts/dispatch_report.py --recent
```

**Axes tracked:** reads, edits, tool total, bridge turns, output tokens, cache tokens, cost USD, wall time, edit efficiency %, cost per violation fixed.

## Traps

- **Missing role aliases** — if audit doesn't detect agent-added blocks, check `roleAliases` in `blocks.go`. English headings (Decisions, Actions, References) need explicit mappings.
- **Stub conversion** — agents may add blocks below min-lines, converting `missing` to `stub`. Add min-lines hint in prompt to prevent.
- **No idempotency** — re-running dispatch without guard creates duplicate sections. Always include "skip if exists" instruction.
- **Batch size > 8** — agents lose focus. Optimal: 4 tasks per batch for semantic work, 8 for mechanical.
- **Bloated fixes need review** — compression may lose information. Run audit after, spot-check 2-3 files.

## References

- `tools/ask/fix.go` — `--dispatch` implementation, prompt generation
- `tools/scripts/dispatch_report.py` — post-experiment analysis
- `tools/scripts/benchmark_fix.sh` — baseline benchmark
- `knowledge/playbook/pattern_agent_tool_design.md` — underlying tool design pattern
