---
id: cross-session-analysis
title: Session Analysis
type: methodology
concern: [context-management, knowledge-architecture]
mechanism: [pipeline, scoring-model]
scope: system
lifecycle: [improve]
origin: harvest/vadim-assistant
maturity: draft
domain: cross-agent/methodologies
confidence: observed
basis: "distilled from vadim-assistant session analysis practice"
---

# Session Analysis

<!-- CORE: load always -->
## Problem

Agent sessions consume tokens without visibility into where the budget actually goes. Cognitive offload rules, tool usage policies, and efficiency guidelines exist as text instructions, but without measurement, enforcement gaps remain invisible. An agent may re-read the same file five times, use bash where a dedicated Grep tool exists, or repeat the same command sequence across sessions -- and nobody knows the magnitude of the waste or the ROI of fixing each pattern.

The problem is self-reinforcing: without data, improvement efforts target the wrong patterns. A developer might spend hours creating a script for an operation that happens twice per month, while ignoring a bash antipattern that burns thousands of tokens daily. Intuition about "what wastes the most tokens" is systematically wrong because humans cannot estimate cumulative costs across hundreds of sessions.

## Solution

A two-level analysis pipeline that runs **outside** the agent's context window, processing raw JSONL session logs into structured JSON reports:

1. **Surface analysis** (`analyze_sessions.py`) -- scans session logs for tool usage counts, bash command categories, repeated operations, antipattern detection (bash used where dedicated tools exist), and offload candidates ranked by frequency.
2. **Deep analysis** (`analyze_sessions_deep.py`) -- performs token-level economics, classifying every tool call by token cost, categorizing enforcement gaps (ENFORCE vs. SCRIPT vs. MIXED vs. ANALYZE), and computing ROI per potential intervention.

Both scripts support multi-project analysis, allowing cross-agent comparison. Results are JSON reports with an `action_required` field, suitable for direct agent consumption.

The analysis feeds back into the cognitive offload loop: patterns with count >= 3 become scripting candidates (Rule of Three), antipattern detections trigger rule enforcement review, and savings percentage guides prioritization. Key health thresholds -- error rate (<3% green), antipattern calls per session (<5 green), re-read ratio (<0.3 green), saveable tokens (<10% green) -- provide clear signals for when intervention is needed.

## Implementation

### Scripts

| Script | Purpose | Output |
|--------|---------|--------|
| `analyze_sessions.py` | Surface patterns: tool counts, repeats, antipatterns | `session_analysis.json` |
| `analyze_sessions_deep.py` | Token economics: waste by category, enforcement ROI | `session_deep_analysis.json` |

Both live in `30_tools/scripts/` and support multi-project analysis.

### Multi-Project Support

```bash
# List all Claude Code projects on this machine
python3 30_tools/scripts/analyze_sessions.py --list-projects

# Analyze specific project
python3 30_tools/scripts/analyze_sessions.py --project voic-experiment --date 2026-03-05

# Auto-detect from cwd
python3 30_tools/scripts/analyze_sessions.py --date 2026-03-05
```

### When to Run

| Trigger | Which Analysis | Why |
|---------|---------------|-----|
| End of day | Surface | Spot new repeated patterns |
| After large session (500+ messages) | Deep | Quantify waste, identify scripting candidates |
| Before creating new script | Surface | Verify the pattern actually repeats (rule of three) |
| Monthly review | Both, all projects | Cross-agent comparison, trend detection |

### How to Interpret Results

#### Surface Analysis

- **`offload_candidates`** — ranked list of mechanical operations to script
  - `repeated-read` (count ≥ 5): file needs indexing or caching
  - `repeated-bash` (count ≥ 3): command should be a script
  - `tool-antipattern`: dedicated tool exists but bash was used
- **`error_rate`** > 5%: something is systematically wrong

#### Deep Analysis

- **`enforcement_analysis.interventions`** — classified by action:
  - `ENFORCE`: rules exist, strengthen enforcement (no new code)
  - `SCRIPT`: genuine repeated workflow, create a script
  - `MIXED`: partially enforceable, partially inherent cost
  - `ANALYZE`: needs classification before deciding
- **`enforcement_analysis.savings_pct`** — total saveable tokens as % of total

### Integration with Cognitive Offload

Session analysis is the **measurement arm** of cognitive offload. The feedback loop:

```
work → session logs → analyze → identify patterns → create scripts → work better
                                                  → strengthen rules
```

Key insight from initial deployment: **enforcement gaps matter more than missing scripts**. Two levels of anti-bash rules already exist (Claude Code system prompt + cognitive-offload CLAUDE.md) but are routinely violated. Adding more rules without measurement won't help. Session analysis provides the measurement.

### Thresholds

| Metric | Green | Yellow | Red |
|--------|-------|--------|-----|
| Error rate | < 3% | 3-5% | > 5% |
| Antipattern calls/session | < 5 | 5-15 | > 15 |
| Re-read ratio (re-reads / total reads) | < 0.3 | 0.3-0.5 | > 0.5 |
| Saveable tokens % | < 10% | 10-20% | > 20% |

<!-- EXTENDED: load on demand -->
## JSONL Format Reference

Claude Code session files are stored in `~/.claude/projects/<project-dir>/*.jsonl`. Each line is an envelope:

```json
{"type": "...", "message": {"role": "assistant|user", "content": [...]}}
```

Content blocks:
- `{"type": "tool_use", "id": "...", "name": "Bash", "input": {"command": "..."}}`
- `{"type": "tool_result", "tool_use_id": "...", "content": "...", "is_error": false}`
- `{"type": "text", "text": "..."}`

Token estimation: ~3.5 chars per token (rough, sufficient for relative comparisons).
