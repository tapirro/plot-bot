---
id: cross-checkpoint-resume
title: Checkpoint & Resume
type: pattern
concern: [resilience, context-management]
mechanism: [checkpoint, state-machine]
scope: per-cycle
lifecycle: [act, reflect]
origin: harvest/spec-creator
origin_findings: [8, 20, 24, 25, 33, 34, 40, 56, 63]
maturity: draft
domain: cross-agent/patterns
confidence: observed
basis: "distilled from spec-creator harvest, 104 findings across 16+ agent sessions"
---

# Checkpoint & Resume

<!-- CORE: load always -->
## Problem

LLM agents lose all in-context state on session interruption or context compaction. This is not a minor inconvenience -- it is a structural failure mode that causes agents to skip pipeline phases, repeat completed work, or follow stale instructions.

Context compaction causes triple information loss. First, task creation compresses a multi-phase pipeline into a simplified summary. Second, compaction further reduces that summary. Third, the agent interprets the compacted text literally, losing domain ordering and gate requirements. A 7-phase pipeline with gate scripts becomes "analyze sources and create YAML cards" -- phases merged, gates lost, position lost.

Plans remembered from context are unreliable. Agents follow compacted summaries instead of actual pipeline state, and they cannot distinguish between "I was told to do X" and "I should do X next." Without external state persistence, every session interruption risks corrupting the entire workflow.

## Solution

Establish disk-based session state as the **sole source of truth** for pipeline position, replacing in-context memory entirely. The core mechanism has four parts:

1. **Structured state file** (`session_state.json`) containing current phase, next phase, pipeline position breadcrumb, and a human-readable `resume_instruction` field that tells the agent exactly what to do next.
2. **Atomic checkpoints** after each logical batch -- a single script call that updates all state files together (session state, processed sources, extraction log, coverage summary). Either all files update or none do.
3. **Batch atomicity rule** -- if a batch has no extraction log entry, it is incomplete and must be replayed. This is safe because all mutations are idempotent.
4. **Forced session split** after N context overflows -- each overflow adds a compaction summary that itself consumes context, accelerating subsequent overflows. A fresh session breaks this vicious cycle.

On resume, the agent reads state files in a defined order, never compaction summaries. When session state conflicts with any in-context memory, session state wins unconditionally.

## Implementation

### Structure

```
SESSION_STATE (session_state.json)
├── phase: string                    # current pipeline phase
├── next_phase: string               # what comes next
├── pipeline_position: string        # machine-readable breadcrumb
├── resume_instruction: string       # human-readable "do this next"
├── batch_id: string                 # current/last batch
├── overflow_count: int              # context overflows this session
├── analysis_iteration: int          # bounded re-analysis counter
└── last_checkpoint: datetime

CHECKPOINT_OPERATION (atomic)
├── 1. Update session_state.json
├── 2. Update processed_sources.json
├── 3. Append to extraction_log.md
└── 4. Update coverage_summary.json

RESUME_READING_ORDER
├── 1. session_state.json            # FIRST: sole source of truth
├── 2. processed_sources.json        # what has been done
├── 3. extraction_log.md             # per-batch results
├── 4. {PIPELINE_PLAN}               # what was planned
└── 5. coverage_summary.json         # current completeness
```

### Checkpoint Atomicity

A checkpoint is a **single operation** that updates all state files together. Either all files are updated or none. This prevents inconsistent state where artifacts exist but indexes are stale, or batches complete but the log is missing entries.

```
BEFORE CHECKPOINT          AFTER CHECKPOINT
┌────────────────┐         ┌────────────────┐
│ session_state:  │         │ session_state:  │
│   phase: analysis│        │   phase: analysis│
│   batch: B-003  │         │   batch: B-004  │
│                 │         │   overflow: 0   │
│ log: B-001,B-002│         │ log: B-001..B-003│
│                 │         │                 │
│ coverage: 45%   │         │ coverage: 62%   │
└────────────────┘         └────────────────┘
 Batch B-003 done but       All state files
 not yet checkpointed       consistent
```

Implemented as one script call (e.g., `checkpoint.py`), not manual sequential steps.

### Batch Atomicity Rule

If a batch has **no extraction_log entry**, it is incomplete. The entire batch must be replayed. This is safe because all mutations are idempotent (append-only or upsert-only). A partially completed batch produces no observable state change until checkpointed.

### Resume Instruction Field

The `resume_instruction` field contains a **human-readable sentence** telling the agent exactly what to do next:

```json
{
  "phase": "analysis",
  "next_phase": "coverage_audit",
  "pipeline_position": "session_start -> pre_extraction -> [analysis] B-004",
  "resume_instruction": "Continue analysis phase. Next batch: B-004. Sources: SRC-045..SRC-052. Load analysis rules before processing.",
  "batch_id": "B-004",
  "overflow_count": 0,
  "last_checkpoint": "2026-03-05T14:32:00Z"
}
```

This field is **redundant** to pipeline rules but survives context compaction. The agent reads one JSON file instead of reconstructing state from scattered documents.

### Triple Information Loss (Context Compaction)

```
ORIGINAL PIPELINE STATE         AFTER COMPACTION
┌──────────────────────────┐    ┌──────────────────────────┐
│ 7 phases with gates      │    │ "analyze sources and     │
│ Phase 4: review           │ →  │  create YAML cards"      │
│ Phase 5: merge           │    │                          │
│ Gate: review_gate.py     │    │ (phases merged,          │
│ Next: run reviewer       │    │  gates lost,             │
│ subagents on cluster C2  │    │  position lost)          │
└──────────────────────────┘    └──────────────────────────┘
```

**Fix:** session_state.json on disk is immune to compaction. The agent reads it on resume and ignores any compaction summary that contradicts it.

### Forced Split Rule

After `{MAX_OVERFLOWS_PER_SESSION}` context overflows in a single session, the agent MUST:

1. Finish the current batch (or mark it incomplete)
2. Execute a full checkpoint
3. Start a completely fresh session using Resume Protocol

**Rationale:** Each context overflow adds a compaction summary that itself consumes context, accelerating subsequent overflows in a vicious cycle. A fresh session starts clean.

```
Session timeline with overflow accumulation:

Overflow 1: +2K tokens (summary)  → 98K usable
Overflow 2: +2K tokens (summary)  → 96K usable (summaries compound)
Overflow 3: +2K tokens (summary)  → 94K usable ← FORCED SPLIT HERE
                                     Next session: 100K usable (clean)
```

### Differential Logging

Each extraction log entry records results of ONLY the current batch: sources processed, artifacts created, context volume consumed. Cumulative totals are **forbidden** -- they make per-batch yield calculation impossible and hinder retrospective analysis.

```markdown
| Batch | Sources | Created | Updated | Context Vol |
|-------|---------|---------|---------|-------------|
| B-001 | 5       | 8       | 0       | 3200 lines  |
| B-002 | 4       | 3       | 2       | 2800 lines  |
```

### Decision Rules

| Situation | Action |
|-----------|--------|
| On resume (new session) | Read session_state.json FIRST, follow `resume_instruction` |
| After completing a batch | Execute atomic checkpoint (all 4 files) |
| Context overflow occurs | Increment `overflow_count`, checkpoint, continue |
| `overflow_count` >= `{MAX_OVERFLOWS_PER_SESSION}` | Checkpoint + start fresh session (forced split) |
| Batch has no log entry | Incomplete -- replay entire batch (safe: idempotent) |
| Compaction summary conflicts with session_state | session_state wins ALWAYS |
| Plans conflict with checkpoints | Checkpoints are factual; plans are aspirational -- follow checkpoints |
| Need to know "what happened" | Read extraction_log (differential), not accumulated summaries |

### Configuration

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `{MAX_OVERFLOWS_PER_SESSION}` | Overflows before forced split | 3 | yes |
| `{STATE_FILE}` | Path to session state JSON | "context/session_state.json" | yes |
| `{LOG_FILE}` | Path to extraction log | "context/extraction_log.md" | yes |
| `{PROCESSED_FILE}` | Path to processed sources | "context/processed_sources.json" | yes |
| `{COVERAGE_FILE}` | Path to coverage summary | "context/coverage_summary.json" | yes |
| `{PIPELINE_PLAN}` | Path to pipeline plan | "context/extraction_plan.md" | no |
| `{CHECKPOINT_SCRIPT}` | Script executing atomic checkpoint | "scripts/checkpoint.py" | yes |
| `{BATCH_ID_FORMAT}` | Batch identifier pattern | "B-{NNN}" | no |

<!-- REFERENCE: load on adoption -->
## Adaptation Guide

### Prerequisites
- Filesystem accessible to the agent for state persistence
- Idempotent mutation policy on all state files (append-only or upsert-only)
- A logical "batch" or "step" concept in the pipeline
- Script infrastructure for atomic checkpoint execution

### Steps to Adopt
1. Define `session_state.json` schema with `phase`, `next_phase`, `pipeline_position`, `resume_instruction`, `overflow_count`
2. Define all context files with explicit merge policies (append-only, upsert-only, overwrite)
3. Implement the atomic checkpoint as a single script that updates all state files
4. Add `resume_instruction` computation to the checkpoint script (human-readable next step)
5. Implement the resume protocol: on session start, read state files in defined order
6. Add `overflow_count` tracking; implement forced split rule at threshold
7. Switch extraction logging to differential (per-batch, not cumulative)
8. Add batch atomicity check: missing log entry = incomplete batch = replay
9. Add explicit instruction: "session_state.json wins over compaction summaries"
10. Test resume by interrupting mid-batch and verifying correct recovery

### What to Customize
- State file paths and names -- match your project's directory conventions
- Fields in session_state.json -- add domain-specific position markers
- Checkpoint script contents -- depends on what state files your pipeline uses
- `{MAX_OVERFLOWS_PER_SESSION}` threshold -- tune based on your context window size
- Batch granularity -- what constitutes one "batch" in your domain
- Log format -- markdown table, JSONL, or structured JSON

### What NOT to Change
- session_state.json as SOLE source of truth -- without this, compaction destroys pipeline state
- Atomic checkpoint (all-or-nothing) -- partial updates create inconsistent state
- Forced split after repeated overflows -- without this, summary accumulation accelerates degradation
- Differential logging (per-batch only) -- cumulative logs make yield analysis impossible
- Resume reads state file FIRST -- reading any other file first risks following stale context
- Batch atomicity (no log = replay) -- without this, partial batches create silent gaps
- Idempotent mutations -- without idempotency, replay after interruption corrupts data

<!-- HISTORY: load for audit -->
## Origin
- **Source agent:** spec-creator
- **Findings:** [8] Resume protocol with checkpoint-based recovery, [20] Context compaction causes triple information loss, [24] Atomic checkpoint pattern, [25] Resume-friendly session state with computed fields, [33] Forced split rule for repeated overflows, [34] Batch atomicity rule, [40] Differential logging (per-batch, not cumulative), [56] Checkpoints over plans principle, [63] Context JSON schemas with explicit merge policies
- **Discovered through:** An incident where context compaction caused the agent to skip the review phase entirely. Post-incident analysis revealed three stages of information loss: task creation compressed the 7-phase pipeline, compaction further simplified it, and the agent read "create YAML cards" literally -- bypassing review. The fix: session_state.json on disk with `next_phase` and `resume_instruction` fields, read before any action on resume. The forced split rule was added after observing that 3+ overflows in one session created a vicious cycle where each compaction summary consumed context, accelerating the next overflow.
- **Evidence:** Triple information loss documented in incident report (2026-02-25). Forced split at 3 overflows eliminated the summary accumulation cycle. Atomic checkpoint prevented inconsistent state across 100+ batches. Differential logging enabled per-batch efficiency metrics that cumulative logging made impossible.

## Related Patterns
- [Subagent Coordination](subagent_coordination.md) -- Disk-First subagent output creates checkpointable intermediate state
- [Phase Gate & Task DAG](phase_gate.md) -- phase gates verify work before allowing checkpoint to advance phase
- [Validation Gates](validation_gates.md) -- checkpoint includes validation step before state update
- [Autonomous Loop Protocol](loop_protocol.md) -- loop iterations checkpoint after each cycle
- [Provider Resilience](provider_resilience.md) -- provider health state persists across checkpoints
- append_only_audit -- checkpoint log is append-only, never edited
- context_budget -- forced split rule directly manages context budget
- cognitive_offload -- checkpoint script offloads state management from agent to code
- knowledge_layers -- session_state is Layer 0 (infrastructure), resume_instruction is Layer 2 (strategy)
- [Cognitive Offload](../methodologies/cognitive_offload.md) -- checkpoint scripts are a key cognitive offload target
- [Knowledge Layer Architecture](../methodologies/knowledge_layers.md) -- session_state is Layer 0; resume_instruction is Layer 2
