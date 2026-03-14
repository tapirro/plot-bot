---
id: cross-subagent-coordination
title: Subagent Coordination
type: pattern
concern: [multi-agent-coordination]
mechanism: [contract, registry]
scope: per-item
lifecycle: [act, reflect]
origin: harvest/spec-creator
origin_findings: [6, 7, 55, 62, 72, 75, 90]
maturity: draft
domain: cross-agent/patterns
confidence: observed
basis: "distilled from spec-creator harvest, 104 findings across 16+ agent sessions"
---

# Subagent Coordination

<!-- CORE: load always -->
## Problem

When subagents return full results through the orchestrator's context window, the system hits a hard scaling wall. Each subagent returning 10-15K tokens of findings means N subagents consume N x 15K tokens of orchestrator context -- a pipeline with 5 subagents processing 100 items can generate 80K+ tokens of results, exceeding the orchestrator's effective working memory and triggering context compaction that loses critical details.

Without a fixed return contract, subagents produce format variations that break downstream processing. Five subagents given the same schema independently produce three different JSON key names for the same field, different nesting structures, and inconsistent array formats. The consuming script either crashes or silently drops data.

Parallel subagents without ID coordination generate colliding identifiers. Two agents independently numbering findings from 1 produce duplicate IDs that overwrite each other during merge. This corruption is silent -- the merge appears to succeed, but half the findings are lost.

## Solution

The Disk-First Rule eliminates context overflow by separating communication channels: subagents write granular findings to disk (one file per analyzed item, written immediately after analysis) and return only a fixed-schema summary to the orchestrator. The summary contains metadata -- batch ID, findings count, file paths, issues encountered -- consuming roughly 0.5-1K tokens instead of the full 10-15K per subagent.

The orchestrator never reads findings into its own context. Instead, it dispatches scripts that process disk-based findings files directly. This keeps the orchestrator's context lean (summaries only) while the complete record lives on disk with no size constraint.

Structured return contracts with schema-by-reference (path to a canonical schema file, not inline examples) eliminate format drift. Subagents read the schema from a single source of truth rather than interpreting inline examples, which they tend to copy literally rather than generalize from.

ID range pre-allocation before dispatch prevents collisions: Subagent A receives IDs 0001-0020, Subagent B receives 0021-0040. No negotiation, no locking, no conflicts. References to not-yet-created IDs are forbidden; cross-references are filled via idempotent upsert after all subagents complete.

Role separation ensures each agent type has a clearly bounded scope: analysts produce findings, reviewers verify and enrich findings, merge agents create final artifacts, and the orchestrator coordinates without reading raw data. This prevents scope creep where a merge agent tries to read original sources or an orchestrator tries to do analysis.

## Implementation

### Structure

```
SUBAGENT_CONTRACT
├── input
│   ├── task_description: string
│   ├── id_range: [start, end]
│   ├── output_directory: path
│   └── schema_reference: path (NOT inline)
├── output_on_disk
│   ├── findings/{ITEM_ID}.json    # one file per analyzed item
│   └── format: defined by schema_reference
└── return_to_orchestrator
    ├── batch_id: string
    ├── domain: string
    ├── findings_count: int
    ├── findings_files: list[path]
    ├── type_distribution: dict
    └── issues: list[string]
```

### Disk-First Rule

All subagent work products are written to disk **incrementally** -- one file per analyzed item, written immediately after analysis. The orchestrator processes disk files via scripts and never reads them into its own context.

```
ORCHESTRATOR CONTEXT (lean)         DISK (complete)
┌─────────────────────────┐         ┌──────────────────────────┐
│ subagent_1_summary:     │         │ findings/                │
│   batch_id: "B-001"     │         │   ITEM-001.json (full)   │
│   findings_count: 12    │         │   ITEM-002.json (full)   │
│   issues: []            │         │   ...                    │
│                         │         │   ITEM-012.json (full)   │
│ subagent_2_summary:     │         │   ITEM-013.json (full)   │
│   batch_id: "B-002"     │         │   ...                    │
│   findings_count: 8     │         │                          │
│   issues: ["1 UNK"]     │         │                          │
└─────────────────────────┘         └──────────────────────────┘
 ~1K tokens total                    ~80K+ tokens if loaded
```

### ID Range Pre-allocation

Before dispatching parallel subagents, the orchestrator assigns non-overlapping ID ranges:

```
Orchestrator allocates:
  Subagent A → {ITEM_TYPE}-0001 .. {ITEM_TYPE}-0020
  Subagent B → {ITEM_TYPE}-0021 .. {ITEM_TYPE}-0040
  Subagent C → {ITEM_TYPE}-0041 .. {ITEM_TYPE}-0060
```

References to not-yet-created IDs are forbidden. If Item X needs a link to Item Y that does not yet exist, the link field is filled later via idempotent upsert.

### Prompt Hygiene Rules

| Rule | Rationale |
|------|-----------|
| No inline format examples in prompts | Subagents copy the example literally instead of following the schema |
| Reference schema by path, not content | Single source of truth; no desynchronization |
| Do not specify output file paths | Subagent derives paths from Disk-First Rule conventions |
| Do not include orchestrator context | Subagent sees only its slice, not the full picture |

### One File Per Item, Not Per Batch

Findings are stored as **one file per analyzed item** (`findings/{ITEM_ID}.json`), not per batch. This enables:
- Selective loading: downstream agents read exactly the files they need
- Partial resume: completed items are not re-processed
- Point merging: merge agent loads 1-2 files, not an entire batch

### Role Separation

| Role | Reads | Produces | Does NOT |
|------|-------|----------|----------|
| Analyst subagent | Source material | `findings/{ITEM_ID}.json` | Create final artifacts |
| Reviewer subagent | Findings + source evidence | `review-{ITEM_ID}.json` | Read script source code |
| Merge agent | Findings + review enrichments | Final artifacts | Read original sources |
| Orchestrator | Summaries only | Dispatch + checkpoints | Read findings into context |

### Decision Rules

| Situation | Action |
|-----------|--------|
| Subagent completes analysis of one item | Write `findings/{ITEM_ID}.json` immediately |
| Subagent completes batch | Return structured summary (~0.5-1K tokens) |
| Orchestrator needs findings detail | Read from disk via script, NOT from context |
| Parallel subagents need IDs | Pre-allocate non-overlapping ranges before dispatch |
| Subagent encounters format question | Read `schema_reference` file, NOT inline example |
| Merge agent hits validation error | Fix input data; do NOT grep script source code |
| Subagent returns unexpected field names | Consuming script uses fallback chain for field resolution |

### Configuration

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `{ITEM_TYPE}` | Prefix for generated item IDs | "FR", "TASK", "FINDING" | yes |
| `{OUTPUT_DIRECTORY}` | Base path for findings files | "context/findings/" | yes |
| `{SCHEMA_REFERENCE}` | Path to canonical output schema | "rules/schemas/finding.yaml" | yes |
| `{MAX_ITEMS_PER_SUBAGENT}` | Maximum items in one subagent slice | 50 | yes |
| `{SUMMARY_TOKEN_BUDGET}` | Maximum tokens in return summary | 1000 | yes |
| `{MAX_PARALLEL_SUBAGENTS}` | Concurrent subagent limit | 2-3 | no |
| `{BATCH_ID_FORMAT}` | Batch identifier pattern | "B-{NNN}" | no |

<!-- REFERENCE: load on adoption -->
## Adaptation Guide

### Prerequisites
- Multi-agent system with orchestrator dispatching work to subagents
- Shared filesystem or artifact storage accessible to all agents
- Structured output schema for subagent results
- Script or tool for processing disk-based findings

### Steps to Adopt
1. Define the output schema for subagent findings (one JSON schema file)
2. Establish the `{OUTPUT_DIRECTORY}` convention for findings files
3. Define the structured return contract (batch_id, count, files, issues)
4. Implement ID range pre-allocation in the orchestrator dispatch logic
5. Remove all inline format examples from subagent prompts; replace with schema path references
6. Switch findings granularity from per-batch to per-item files
7. Add a consuming script that reads findings from disk (not orchestrator context)
8. Define role separation: which agent reads what, produces what
9. Add format tolerance in consuming scripts (fallback field name chains)
10. Test with a workload large enough to trigger context pressure (50+ items)

### What to Customize
- Item ID format and prefix (`{ITEM_TYPE}`) -- match your domain's naming convention
- Output directory structure -- flat or nested by domain/batch
- Summary contract fields -- add domain-specific fields (e.g., type_distribution)
- Maximum items per subagent slice -- tune to your context budget
- Role definitions -- your pipeline may have different agent roles than analyst/reviewer/merge

### What NOT to Change
- Disk-First Rule -- returning full results through context guarantees overflow at scale
- One file per item granularity -- batch files force loading irrelevant data
- Structured return contract -- free-form returns make orchestrator planning impossible
- ID pre-allocation for parallel agents -- without it, collisions are inevitable
- Schema by reference, not inline -- inline examples cause format drift
- Role separation (merge agent does not read sources) -- mixing concerns degrades quality

<!-- HISTORY: load for audit -->
## Origin
- **Source agent:** spec-creator
- **Findings:** [6] Disk-First Rule for subagent output, [7] Structured return contract with fixed JSON format, [55] Prohibition on inline format examples, [62] One finding file per card, [72] ID range pre-allocation, [75] Merge agent should not read source code, [90] Subagent contract evolution through empirical observation
- **Discovered through:** A workspace with 108 sources caused 80K+ token context overflow when subagent materialization files and TaskOutput were read into orchestrator context. JSONL log forensics revealed the root cause was not "too many agents" but reading full results into context. The Disk-First Rule eliminated overflow. Format drift was discovered when 5 subagents produced 3 different JSON key names for the same array. The prohibition on inline examples was added after subagents copied example format verbatim instead of following the schema. One-file-per-card granularity replaced batch files after merge agents were forced to load 25+ card files to process one card.
- **Evidence:** 108 sources processed without context overflow after Disk-First adoption. ~80K tokens saved per pipeline transition. Format tolerance handles 3 field name variations. Subagent contract evolved through 6 versions (v1-v4.2), each driven by log forensics of production runs.

## Related Patterns
- [Checkpoint & Resume](checkpoint_resume.md) -- subagent disk output enables checkpoint-based recovery
- [Phase Gate & Task DAG](phase_gate.md) -- phase gates verify subagent work was actually performed
- [Provider Resilience](provider_resilience.md) -- subagents calling external providers benefit from fallback chains
- [Validation Gates](validation_gates.md) -- post-write validation catches subagent format errors
- [Autonomous Loop Protocol](loop_protocol.md) -- orchestrator loop consumes subagent summaries as feedback
- context_budget -- Disk-First directly addresses context budget by keeping orchestrator lean
- cognitive_offload -- scripts process disk findings, offloading mechanical work from agents
- append_only_audit -- findings files preserved after final artifact creation form audit trail
- analysis_review_merge -- role separation between analyst, reviewer, and merge agents
- [Adversarial Reflection](../patterns/adversarial_reflection.md) -- adversarial analyst/reviewer roles map to subagent role separation
- [Cognitive Offload](../methodologies/cognitive_offload.md) -- scripts process disk findings, offloading mechanical work from agents
