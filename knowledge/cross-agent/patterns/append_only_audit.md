---
id: cross-append-only-audit
title: Append-Only Audit Trail
type: pattern
concern: [data-integrity]
mechanism: [registry]
scope: per-item
lifecycle: [act, reflect]
origin: harvest/spec-creator
origin_findings: [2, 51, 58, 59, 68, 76]
maturity: draft
domain: cross-agent/patterns
confidence: observed
basis: "distilled from spec-creator harvest, 104 findings across 16+ agent sessions"
---

# Append-Only Audit Trail

<!-- CORE: load always -->
## Problem

Long-running agent pipelines produce records (gaps, unknowns, findings, logs) that accumulate across sessions. These records form the pipeline's institutional memory: what was attempted, what failed, what remains unresolved. Deleting or overwriting records destroys audit trails, breaks idempotent replay on resume, and makes retrospective analysis impossible.

Without explicit mutation policies, agents default to "helpful cleanup" -- removing resolved items, overwriting stale entries, or compacting logs into summaries. Each of these well-intentioned actions silently corrupts pipeline history. The damage is invisible at the time: it surfaces later when a session crashes mid-batch and replay produces duplicates, or when a retrospective tries to compute per-batch yield and finds only cumulative totals that cannot be decomposed.

The problem compounds in multi-session pipelines where context is lost between sessions. The only reliable source of continuity is what was written to disk, and if that record has been mutated in place, the pipeline's ability to resume, audit, and learn from its own history is fundamentally compromised.

## Solution

Every registry declares an explicit merge policy -- append-only, upsert-only, or derived -- stated in the file's header, not implied by convention. This declaration is the single source of truth for how any agent may mutate the file.

Records are never deleted. Instead, they transition to terminal states (`resolved`, `deprecated`) with timestamps and reasons. This preserves the full lifecycle: when an item was created, when it was resolved, and why. The historical record enables trend analysis and pattern detection across runs.

Each artifact declares its source of truth authority (SELF or EXTERNAL), preventing downstream confusion about what is canonical versus derived. When two files disagree, the authority declaration resolves the conflict mechanically.

Batch logs record only differentials -- this batch's delta (created, modified, linked) -- not cumulative totals. This ensures per-batch yield is always computable and retrospective analysis can reconstruct the full timeline from individual entries. The combination of append-only and upsert-only policies makes all operations safe to replay: crash at any point, resume by replaying the last batch, and the result is identical to an uninterrupted run.

## Implementation

### Structure

```
REGISTRY_FILE
├── path: string
├── merge_policy: APPEND_ONLY | UPSERT_ONLY | DERIVED
├── source_of_truth: SELF | EXTERNAL({AUTHORITY_REF})
├── schema: reference
├── deletion: NEVER (transition to resolved/deprecated)
└── conflict_resolution: "latest write wins" for upsert, "append" for append

AUDIT_RECORD
├── id: string (monotonic, never reused)
├── status: active | resolved | deprecated
├── created_at: datetime
├── resolved_at: datetime | null
├── resolution_reason: string | null
└── content: varies by registry type

BATCH_LOG_ENTRY
├── batch_id: string
├── timestamp: datetime
├── delta
│   ├── created: list[id]
│   ├── modified: list[id]
│   └── linked: list[{from, to}]
└── totals_snapshot: dict (optional, for verification)

FINDINGS_TRAIL
├── stage: analysis | review | merge
├── findings_files: list[path]
├── preserved_after_final_output: true (NEVER deleted)
└── purpose: provenance chain from raw input to final artifact
```

### Configuration

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `{REGISTRY_PATH}` | File path for the registry | `context/gaps.json` | yes |
| `{MERGE_POLICY}` | Mutation policy for the file | `APPEND_ONLY`, `UPSERT_ONLY`, `DERIVED` | yes |
| `{AUTHORITY_REF}` | Primary source reference for EXTERNAL SoT | `SRC-0042`, `API endpoint` | if EXTERNAL |
| `{ID_PREFIX}` | Prefix for record IDs | `GAP-`, `UNK-`, `SRC-` | yes |
| `{TERMINAL_STATES}` | States that mark a record as closed | `resolved, deprecated` | yes |
| `{LOG_FORMAT}` | Format for batch log entries | `JSONL`, `JSON array` | recommended |

### Merge Policy Table

| Registry Type | Policy | Rationale |
|--------------|--------|-----------|
| Gaps / unknowns / no-artifact | APPEND_ONLY | Historical record of what was missing and when |
| Session state / coverage | UPSERT_ONLY | Current state matters; history in logs |
| Indexes / summaries | DERIVED | Regenerated from source cards; never hand-edited |
| Extraction log | APPEND_ONLY | Each entry = one batch delta; immutable after write |
| Findings (analyst/reviewer) | APPEND_ONLY | Preserved as provenance trail after merge |

### Decision Rules

```
FOR each registry write operation:
  1. What is the file's declared merge_policy?
     → APPEND_ONLY → add new record, never modify existing
     → UPSERT_ONLY → update existing record by ID, add if new
     → DERIVED → regenerate entire file from source data

  2. Is the operation a deletion?
     → ALWAYS REJECT
     → Instead: set status to resolved/deprecated with timestamp

  3. Is this a batch log entry?
     → Record ONLY this batch's delta (created/modified/linked)
     → Do NOT write cumulative totals as primary data
     → Optionally include totals_snapshot for verification

  4. Does this artifact declare source of truth?
     → SELF → this file is canonical; validate against its own schema
     → EXTERNAL → validate against primary source; flag divergence

  5. Are findings files from a completed stage?
     → PRESERVE (do not delete after card/artifact creation)
     → They form the audit trail from input to output
```

### Idempotent Replay Guarantee

The combination of append-only and upsert-only policies makes all operations safe to replay:

```
REPLAY_SAFETY
├── append-only registry:
│   ├── replay appends same record → duplicate detected by ID → skip
│   └── records have monotonic IDs → out-of-order replay is harmless
├── upsert-only registry:
│   ├── replay writes same value → no change (idempotent)
│   └── replay writes newer value → latest wins (safe)
├── derived files:
│   ├── replay regenerates from source → identical output
│   └── no state to corrupt
└── net effect: crash at any point → resume replays last batch → safe
```

This is critical for agent pipelines interrupted by context overflow, timeout, or crash. The Resume Protocol replays the last incomplete batch without risk of data corruption or duplication.

### Anti-Patterns

| Anti-Pattern | Consequence | Fix |
|-------------|-------------|-----|
| Deleting resolved records | Audit trail destroyed; retrospective impossible | Transition to `resolved` status |
| Cumulative totals in batch logs | Per-batch yield uncomputable; hinders retrospective | Differential logging: record only this batch's delta |
| Deleting findings after card creation | Provenance chain broken; no recovery on overflow | Preserve findings indefinitely |
| Manual index editing | Index diverges from actual cards | DERIVED policy: regenerate from source |
| No merge policy declaration | Agents guess; inconsistent mutations | Explicit merge_policy field in every registry |
| Overwriting session state without log | State changes untraceable | Upsert state + append log entry |

### Source of Truth Declaration

Every artifact or registry file includes a source of truth header:

```
SOURCE_OF_TRUTH
├── authority: SELF | EXTERNAL
├── primary_source: string | null (path or reference if EXTERNAL)
├── evidence_links: list[{EVIDENCE_ID}] (SRC-*, finding references)
└── last_verified: datetime
```

This prevents downstream confusion about what is canonical versus derived. When two files disagree, the one with `SELF` authority wins; for `EXTERNAL`, the referenced primary source is consulted.

<!-- REFERENCE: load on adoption -->
## Adaptation Guide

### Prerequisites
- A registry or collection of records that persists across agent sessions
- Identifiable record types with unique IDs
- A need for auditability or safe resume after interruption

### Steps to Adopt
1. Inventory all files the agent writes to during pipeline execution
2. For each file, assign a merge policy: `APPEND_ONLY`, `UPSERT_ONLY`, or `DERIVED`
3. Add a `source_of_truth` declaration to each file's header or schema
4. Replace any delete operations with status transitions (`resolved`, `deprecated`)
5. Ensure batch logs record differentials, not cumulative totals
6. Add a rule: findings/intermediate files are preserved after final output creation
7. Verify idempotency: replaying the last batch produces no duplicates or corruption

### What to Customize
- Record types and their ID schemes (`{ID_PREFIX}`)
- Terminal states beyond `resolved` / `deprecated`
- Specific merge policies per file (the assignment, not the mechanism)
- Log format and storage location
- Evidence link conventions (`SRC-*`, `REF-*`, etc.)

### What NOT to Change
- The prohibition on deletion (transition, never remove)
- Differential logging per batch (not cumulative)
- Source of truth declaration on every persistent file
- Preservation of intermediate work products (findings) after final output
- Idempotency guarantee: append-only + upsert-only = safe replay

<!-- HISTORY: load for audit -->
## Origin

- **Source agent:** spec-creator
- **Findings:** [2] Append-Only/Upsert-Only Mutation Policy, [51] Differential Logging, [58] Context JSON Schemas with Merge Policies, [59] Findings as Audit Trail, [68] Append-Only Audit Trail (methods), [76] Checkpoint as Atomic Multi-Step Operation
- **Discovery story:** The spec-creator pipeline manages 7 context files across multi-session extraction runs. Early versions allowed overwrites and deletions, causing state corruption on resume and making retrospective analysis impossible. After two incidents where batch logs recorded cumulative totals (making per-batch yield calculation "impossible and hindering retrospective"), the system adopted strict differential logging. Separately, findings files were initially deleted after YAML card creation, destroying the provenance chain. Preserving them enabled recovery after context overflow via `extract_findings.py` and retrospective analysis of how artifacts evolved.
- **Evidence:** 7 context files each with documented merge policy. Extraction log switched from cumulative to differential format after retrospective showed per-batch metrics were uncomputable. Findings preservation enabled `extract_findings.py` recovery tool. Idempotent upsert validated: 108 SRC merged, re-merged -- +0 added, 108 skipped.

## Related Patterns
- [Validation Gates](../patterns/validation_gates.md) -- gates verify structural integrity of append-only registries
- [Loop Protocol](../patterns/loop_protocol.md) -- bounded retry loops operate safely with idempotent append/upsert
- [Closed-Loop Quality](../methodologies/closed_loop_quality.md) -- quality tracking relies on preserved audit records
- [Monitoring Principles](../best_practices/monitoring_principles.md) -- differential logs feed monitoring metrics
- [Context Budget Management](../methodologies/context_budget.md) -- differential logging reduces log-reading context cost
- [RaP Methodology](../methodologies/rap_methodology.md) -- logs as primary asset requires append-only preservation
- Checkpoint & Resume (cross-checkpoint-resume) -- idempotent replay depends on append-only guarantees
- Phase Gate (cross-phase-gate) -- gate scripts read registries to verify real work was performed
- Knowledge Layers (cross-knowledge-layers) -- merge policies are Layer 0 infrastructure
- Invariant Testing (cross-invariant-testing) -- "no record deletion" is a structural invariant
- [Knowledge Layer Architecture](../methodologies/knowledge_layers.md) -- merge policies are Layer 0 infrastructure in the knowledge architecture
