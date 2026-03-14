---
id: cross-context-budget
title: Context Budget Management
type: methodology
concern: [context-management]
mechanism: [gate-pyramid, registry]
scope: system
lifecycle: [decide, act]
origin: harvest/spec-creator
origin_findings: [3, 9, 54, 61, 69, 86]
maturity: draft
domain: cross-agent/methodologies
confidence: observed
basis: "distilled from spec-creator harvest, 104 findings across 16+ agent sessions"
---

# Context Budget Management

<!-- CORE: load always -->
## Problem

LLM agents operate within a fixed context window that serves as both working memory and instruction storage. Loading all reference documents at session start -- a natural default behavior -- wastes 60-80% of the budget on documents that may never be used during that session.

The waste compounds through three mechanisms. First, upfront loading: all schemas loaded at session start costs 10-15K tokens even when only one schema is needed. Second, re-reads: agents re-read the same state file 3-5 times per session because the previous content has scrolled out of attention. Third, manual operations: agents performing validation and counting manually consume 15-20 tool calls per batch, each adding content to the context window.

Without a loading strategy, the agent runs out of context mid-task, triggering compaction. Compaction destroys pipeline state, causes the agent to lose its position, and produces a vicious cycle where compaction summaries themselves consume context, accelerating the next compaction event.

## Solution

A **three-tier loading strategy** that defers document loading to the latest responsible moment, combined with disciplines that prevent context waste:

1. **GATE tier** -- mandatory documents loaded at session start or phase transition. These are pipeline rules, session state, and resume protocol. Fixed cost of ~5-8K tokens.
2. **LAZY tier** -- documents loaded on first use within a phase. Schemas, checklists, and contracts are loaded only when the agent first needs them, not upfront. Reduces schema loading from 10-15K to ~1.5-2K per schema.
3. **ON_DEMAND tier** -- documents loaded by explicit agent decision. Full artifact bodies, source files, and evidence documents are loaded only when directly needed.

Supporting disciplines enforce the strategy:
- **Single-read rule**: each document is read once per session. Re-reads are flagged as context waste (exception: resume protocol in new sessions).
- **Lite header / full body protocol**: for artifact collections, the agent navigates by headers (~3 lines each) and loads full bodies only when needed. 100 artifacts navigated by header = ~300 lines vs. ~8000 lines for full bodies (26x reduction).
- **Script-over-read rule**: if extracting information from a document costs more tokens than a script call that computes the same answer, prefer the script.
- **Budget accounting**: token expenditure is tracked across categories (instructions, gate documents, lazy documents, working data) with an efficiency metric of artifacts produced per context tokens consumed.

## Implementation

### Structure

```
DOCUMENT_REGISTRY
├── documents: list[DOC_ENTRY]
│   ├── path: string
│   ├── loading: GATE | LAZY | ON_DEMAND
│   ├── gate_phase: string | null (for GATE docs)
│   ├── estimated_tokens: int
│   └── load_condition: string (when to load)
└── budget
    ├── total_context: int ({CONTEXT_WINDOW_SIZE} tokens)
    ├── reserved_for_output: int ({OUTPUT_RESERVE} tokens)
    ├── reserved_for_state: int ({STATE_RESERVE} tokens)
    └── available_for_docs: int (computed)

ARTIFACT_HEADER (Lite)
├── id: string
├── summary: string (1 sentence)
├── load_full_if: string (conditions for full load)
└── tags: list[string]

ARTIFACT_BODY (Full)
├── header: ARTIFACT_HEADER
├── content: string (complete artifact)
└── estimated_tokens: int

LOADING_LOG (per session)
├── session_id: string
├── loaded: list[{path, phase, tokens, timestamp}]
├── total_tokens_loaded: int
└── re_reads: list[{path, reason}] (should be empty)
```

### Configuration

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `{CONTEXT_WINDOW_SIZE}` | Total context window in tokens | `128000`, `200000` | yes |
| `{OUTPUT_RESERVE}` | Tokens reserved for agent output | `16000` | yes |
| `{STATE_RESERVE}` | Tokens reserved for pipeline state | `8000` | yes |
| `{GATE_DOCS}` | List of mandatory documents per phase | `[rules.md, state.json]` | yes |
| `{HEADER_LINE_LIMIT}` | Max lines for lite header | `3-5` | recommended |
| `{FULL_BODY_THRESHOLD}` | Max tokens before splitting artifact | `500` | recommended |
| `{SCRIPT_ALTERNATIVE_THRESHOLD}` | Token cost above which to prefer script | `5000` | recommended |

### Loading Tiers

| Tier | When Loaded | Examples | Budget Impact |
|------|-------------|---------|---------------|
| GATE | Session start or phase transition | Pipeline rules, session state, resume protocol | ~5-8K tokens total |
| LAZY | First use within a phase | YAML schemas, subagent contracts, checklists | ~1.5-2K per schema |
| ON_DEMAND | Explicit agent decision | Full artifact bodies, source files, evidence docs | Varies |

### Decision Rules

```
FOR each document needed:
  1. Is it already loaded in this session?
     → YES → use cached content, do NOT re-read
     → NO → continue

  2. Is it a GATE document for current phase?
     → YES → load immediately (mandatory)
     → NO → continue

  3. Is it needed for the current operation?
     → YES → load now (lazy loading)
     → NO → defer until needed

  4. Would loading exceed available context budget?
     → YES → consider alternatives:
         a. Can a script extract the needed information?
         b. Can the agent work with the lite header only?
         c. Can the document be summarized to fit?
     → NO → load normally

  5. Is this a re-read of a previously loaded document?
     → Flag as context waste
     → Exception: Resume Protocol (new session) requires re-reading state
```

### Lite Header / Full Body Protocol

For any collection of artifacts exceeding `{FULL_BODY_THRESHOLD}` tokens per item:

```
NAVIGATION MODE:
  Agent reads ONLY headers (~3 lines each)
  Headers contain load_full_if conditions
  Example header:
    id: FR-0042
    summary: "User can filter cohorts by date range and labels"
    load_full_if: "implementing date filtering or label search"

DETAIL MODE:
  Triggered by load_full_if match or explicit need
  Agent loads ONE full body at a time
  Full body replaces header in working memory (not additive)
```

Context savings: navigating 100 artifacts by header = ~300 lines vs. ~8000 lines for full bodies (26x reduction).

### Context Budget Accounting

Track token expenditure across categories:

```
BUDGET_REPORT
├── instructions: {INSTRUCTION_TOKENS} tokens (target: minimize)
├── gate_documents: {GATE_TOKENS} tokens (fixed cost per phase)
├── lazy_documents: {LAZY_TOKENS} tokens (variable, on-demand)
├── working_data: {DATA_TOKENS} tokens (artifacts being processed)
├── output_reserve: {OUTPUT_RESERVE} tokens (reserved)
└── available: computed (total - sum of above)

EFFICIENCY_METRIC
├── artifact_density: artifacts_produced / context_tokens_consumed
├── target: >= 1 artifact per 2K tokens
└── action_if_low: investigate script alternatives, reduce re-reads
```

### Anti-Patterns

| Anti-Pattern | Cost | Fix |
|-------------|------|-----|
| Load all schemas at session start | ~10-15K wasted tokens | Lazy load: ~1.5-2K per schema as needed |
| Re-read state.json multiple times | 2-5x context waste | Single-read rule: read once, work from memory |
| Agent manually counts/validates | 15-20 tool calls per batch | Script: 1 call, deterministic result |
| Full artifact bodies in navigation | ~80 lines per card | Lite headers: ~3 lines per card |
| Cumulative totals in logs | Entire log re-read per batch | Differential logging: read last entry only |

<!-- REFERENCE: load on adoption -->
## Adaptation Guide

### Prerequisites
- A set of reference documents the agent consults during execution
- Measurable context window size for the target LLM
- Ability to split documents into header and body sections

### Steps to Adopt
1. Inventory all documents the agent reads during a typical session
2. Estimate token cost per document (rough: 1 line ~ 10-15 tokens)
3. Classify each as GATE (mandatory) / LAZY (on first use) / ON_DEMAND (rare)
4. For artifact collections, implement lite header / full body split
5. Add `load_full_if` conditions to each header
6. Create a document registry with loading rules
7. Establish the single-read rule: mark which documents are read-once
8. Measure: before/after instruction token counts, tool calls per operation
9. Iterate: if artifact density < 1/2K tokens, investigate script alternatives

### What to Customize
- Document inventory and their tier assignments
- Token budget allocation across categories
- Header format and `load_full_if` conditions
- Artifact density targets (domain-dependent)
- Script alternative thresholds

### What NOT to Change
- Three-tier loading hierarchy (GATE / LAZY / ON_DEMAND)
- Single-read discipline (read once per session, except on resume)
- Lite header / full body separation principle
- Budget accounting as a conscious planning activity
- The rule: if a document costs more tokens than a script call, prefer the script

<!-- HISTORY: load for audit -->
## Origin

- **Source agent:** spec-creator
- **Findings:** [3] Source of Truth Declaration, [9] Lazy Loading of Rules and Schemas, [54] Context Budget Accounting (lessons), [61] Lazy Loading for Review Sub-Agents, [69] Single-Read Rule (methods), [86] Context Budget Strategies (methods)
- **Discovery story:** The spec-creator system initially loaded all 7 YAML schemas at session start (~10-15K tokens), re-read state files 3-5 times per session, and had agents manually performing validation and counting. Cognitive offload analysis revealed ~60% of pre-extraction work, ~50% of each batch, and 100% of coverage audit was mechanical context consumption. Systematic optimization reduced instructions from 25K to 8K tokens (68% reduction), tool calls per batch from 15-20 to 4-6, and introduced lazy SRC loading for reviewer sub-agents (74% of cards never needed source files). Performance baselines showed 67.2 sec/card before lazy loading, with a forecast of 30-40 sec/card after.
- **Evidence:** Instruction tokens: 25K -> 8K (68% reduction). Tool calls per batch: 15-20 -> 4-6 (70% reduction). Schema loading: 10-15K upfront -> 1.5-2K per schema on demand. Reviewer sub-agents: 74% of cards (FR/BR/BM types) never required SRC file loading. Validation errors: probabilistic (agent judgment) -> deterministic (zero misses via scripts).

## Related Patterns
- [Append-Only Audit Trail](../patterns/append_only_audit.md) -- differential logging reduces log-reading context
- [Provider Resilience](../patterns/provider_resilience.md) -- fallback chains add context cost; budget accordingly
- [Validation Gates](../patterns/validation_gates.md) -- gate documents are GATE-tier by definition
- [Monitoring Principles](../best_practices/monitoring_principles.md) -- artifact density is a monitoring metric
- [RaP Methodology](../methodologies/rap_methodology.md) -- algorithmization reduces instruction token cost
- [Scenario-First Testing](../methodologies/scenario_first_testing.md) -- pre-generated test data avoids context-heavy exploration
- Cognitive Offload (cross-cognitive-offload) -- scripts replace context-consuming manual operations
- Knowledge Layers (cross-knowledge-layers) -- Layer 0 code replaces Layer 2 text instructions
- Checkpoint & Resume (cross-checkpoint-resume) -- resume reads minimal state, not full history
- Subagent Coordination (cross-subagent-coordination) -- disk-first rule keeps orchestrator context lean
- [Cognitive Offload](../methodologies/cognitive_offload.md) -- scripts replace context-consuming manual operations
- [Knowledge Layer Architecture](../methodologies/knowledge_layers.md) -- Layer 0 code replaces Layer 2 text instructions, reducing context cost
