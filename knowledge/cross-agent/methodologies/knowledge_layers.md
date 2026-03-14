---
id: cross-knowledge-layers
title: Knowledge Layer Architecture
type: methodology
concern: [knowledge-architecture]
mechanism: [layered-architecture, taxonomy]
scope: system
lifecycle: [improve]
origin: harvest/spec-creator
origin_findings: [15, 16, 22, 57, 64, 74]
maturity: draft
domain: cross-agent/methodologies
confidence: observed
basis: "distilled from spec-creator harvest, 104 findings across 16+ agent sessions"
---

# Knowledge Layer Architecture

<!-- CORE: load always -->
## Problem

Agent systems accumulate knowledge in unstructured text instructions that grow unboundedly as the system matures. A monolithic instruction file mixes stable infrastructure rules (unchanged in weeks) with volatile per-run configuration (different every session), loading all of it into context every time.

This creates three compounding problems. First, context waste: the agent loads 25K tokens of instructions when only 8K are relevant to the current task. Second, staleness: copies of volatile knowledge in stable instruction files become outdated while the source evolves, but the agent follows the stale copy. Third, knowledge leakage: when an executor agent sees the designer's rationale for a rule, it uses that understanding to bypass the rule in edge cases -- the opposite of the intended effect.

The root cause is treating all knowledge as having the same lifecycle. Validators that have not changed in weeks sit next to batch strategies that change every run, formatted identically, loaded identically, with no signal about which is stable and which is volatile.

## Solution

Stratify knowledge into **four layers by rate of change**, each formalized at the appropriate level:

- **Layer 0 (Infrastructure):** Pure code -- scripts, validators, state managers. Changes rarely. Formalized as executable programs.
- **Layer 1 (Domain Model):** Schemas, entity definitions, relationship maps. Changes when the domain expands. Formalized as Pydantic models or JSON schemas.
- **Layer 2 (Strategy):** Heuristics, extraction patterns, batch strategies. Changes often. Formalized as YAML/TOML/markdown configs.
- **Layer 3 (Instance):** Per-run overrides, thresholds, source-specific settings. Changes every session. Formalized as project config files.

Knowledge migrates **downward only** as patterns stabilize (text to config to schema to code). Upward migration is a deliberate rollback signal.

Instructions are split into three abstraction layers loaded progressively: **Why** (objectives, constraints -- always loaded), **What** (process steps, schemas -- loaded on first task of type), and **How** (examples, edge cases -- loaded on failure only). This reduced per-session instruction load from 25K to 8K tokens.

Executor and developer roles are mutually exclusive: the executor sees rules and schemas but not design rationale; the developer sees everything. This prevents knowledge leakage where understanding rationale leads to rule circumvention.

## Implementation

### Structure

```
KNOWLEDGE_LAYERS (by rate of change)
|-- Layer 0: Infrastructure (code)
|   |-- change_rate: rare
|   |-- formalization: pure code (Python, scripts)
|   +-- examples: validators, state managers, index generators
|-- Layer 1: Domain Model (config)
|   |-- change_rate: when domain expands
|   |-- formalization: Pydantic models, JSON schemas
|   +-- examples: artifact types, relationships, required fields
|-- Layer 2: Strategy (flexible config)
|   |-- change_rate: often
|   |-- formalization: YAML/TOML/markdown configs
|   +-- examples: heuristics, extraction patterns, batch strategies
+-- Layer 3: Instance (per-run)
    |-- change_rate: every run
    |-- formalization: project config, overrides
    +-- examples: thresholds, source-specific settings, overrides

INSTRUCTION_LAYERS (by abstraction)
|-- Layer 1 (Why): objectives, constraints, principles
|   +-- loading: ALWAYS (every session)
|-- Layer 2 (What): artifact types, process steps, tools
|   +-- loading: on first task of type
+-- Layer 3 (How): examples, edge cases, error handling
    +-- loading: on failure or complex case only

ROLE_ISOLATION
|-- executor
|   |-- context_budget: {EXECUTOR_BUDGET} tokens (3-5K)
|   |-- sees: rules, schemas, scripts, process steps
|   +-- does_not_see: methodology, metrics, design rationale
+-- developer
    |-- context_budget: {DEVELOPER_BUDGET} tokens (5-8K)
    |-- sees: everything including methodology, metrics, logs
    +-- purpose: improve the system, not execute tasks
```

### Knowledge Migration Direction

Knowledge migrates **DOWNWARD only** as patterns stabilize:

```
Layer 3 (text/overrides)
  -> Layer 2 (YAML/TOML config)
    -> Layer 1 (Pydantic models, schemas)
      -> Layer 0 (pure code, scripts)
```

Upward migration (code -> config -> text) is a **deliberate rollback signal**: the rule was wrong and needs rethinking at a higher abstraction level. This is not failure -- it is learning.

### Instruction Layer Loading Protocol

| Layer | Content | When to Load | Token Budget |
|-------|---------|-------------|-------------|
| Layer 1 (Why) | Objectives, constraints, quality principles | Always, every session | {L1_BUDGET} |
| Layer 2 (What) | Artifact types, process steps, tooling | First task of that type | {L2_BUDGET} |
| Layer 3 (How) | Examples, edge cases, error handling | On failure or complex case | {L3_BUDGET} |

Anti-pattern: duplicating Layer 3 content in Layer 1. This **guarantees desynchronization** -- the copy in Layer 1 becomes stale while Layer 3 evolves.

### Role Isolation

The same LLM agent operates in two **mutually exclusive** roles:

| Aspect | Executor | Developer |
|--------|----------|-----------|
| Purpose | Execute tasks | Improve the system |
| Context budget | {EXECUTOR_BUDGET} tokens | {DEVELOPER_BUDGET} tokens |
| Sees | Rules, schemas, scripts, steps | Everything + methodology, metrics, logs |
| Does NOT see | Why rules exist, design rationale | (no restrictions) |
| Risk if mixed | Knowledge leakage: executor bypasses rules | Scope creep: developer starts executing |

Isolation enforced via `{ROLE_IGNORE_FILE}` (analogous to `.gitignore`) that makes developer files invisible to the executor context.

### Narrative Before Normative

Every instruction file follows this section order:

1. **Purpose** (1 sentence) -- what this file is about
2. **How It Works** (3-10 sentences) -- narrative mental model ("why")
3. **Architecture** (tables/trees) -- types, relations, structure
4. **Rules** (DO/DON'T list) -- one sentence each, verifiable
5. **Procedures** (step-by-step) -- for specific operations

Each section is self-contained and readable independently. Without narrative, rules become constraints without logic and the agent applies them mechanically, mishandling edge cases.

### Ideas Buffer

Deferred improvement ideas stored in `{IDEAS_FILE}` separately from active instructions:

- Each entry: context, concept, unresolved problems, dependencies, "when to implement"
- Ideas are never deleted -- when realized, annotated with "implemented in {WAVE_ID}"
- Reviewed at each planning cycle; relevant items pulled into next wave
- Prevents idea loss while keeping execution context clean

### Configuration

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `{EXECUTOR_BUDGET}` | Max context tokens for executor role | 4000 | yes |
| `{DEVELOPER_BUDGET}` | Max context tokens for developer role | 7000 | yes |
| `{L1_BUDGET}` | Token budget for Layer 1 instructions | 2000 | yes |
| `{L2_BUDGET}` | Token budget for Layer 2 instructions | 3000 | yes |
| `{L3_BUDGET}` | Token budget for Layer 3 instructions | 5000 | no |
| `{ROLE_IGNORE_FILE}` | File that hides dev context from executor | .executorignore | yes |
| `{IDEAS_FILE}` | Path for deferred improvement ideas | docs/future-ideas.md | yes |
| `{NAV_MAP_FILE}` | Q&A navigation map (Layer 2 index) | docs/nav-map.md | yes |
| `{MAX_LINK_HOPS}` | Max hops to answer any question | 3 | yes |

### Decision Rules

```
FOR knowledge placement:

  1. Is this knowledge stable across 10+ runs?
     -> YES -> candidate for Layer 0 (code) or Layer 1 (config)
     -> NO  -> stays at Layer 2 (strategy) or Layer 3 (instance)

  2. Is this a per-project override?
     -> YES -> Layer 3 (instance config)
     -> NO  -> check stability for Layer 0-2

  3. Has this text instruction been stable for 5+ iterations?
     -> YES -> migrate DOWN one layer (text -> config -> model -> code)
     -> NO  -> keep at current layer

FOR instruction loading:

  4. Is this the agent's first action in a session?
     -> Load Layer 1 ONLY (objectives, constraints)

  5. Is this the first task of a new type?
     -> Load Layer 2 for that type (process, schemas, tools)

  6. Did the agent fail or encounter an edge case?
     -> Load Layer 3 (examples, error handling, edge cases)

FOR role selection:

  7. Is the current task "execute a workflow"?
     -> Executor role (minimal context, no methodology)

  8. Is the current task "analyze logs / improve system / review metrics"?
     -> Developer role (full context including methodology)

  9. NEVER mix roles in a single session.
```

<!-- REFERENCE: load on adoption -->
## Adaptation Guide

### Prerequisites
- An agent system with growing instruction files (>10K tokens)
- Identifiable separation between stable and volatile knowledge
- A way to control which files the agent loads (file structure or ignore mechanism)

### Steps to Adopt
1. Audit existing instructions: classify each section by rate of change (rare/sometimes/often/every-run)
2. Create four directories or file prefixes mapping to Layers 0-3
3. Migrate knowledge to appropriate layers, starting with the most stable (Layer 0)
4. Split instruction content into Why/What/How sections
5. Create a navigation map (`{NAV_MAP_FILE}`) for Layer 2 -> Layer 3 references
6. Define executor and developer file sets
7. Create `{ROLE_IGNORE_FILE}` to enforce role isolation
8. Add narrative sections ("How It Works") before rules in all instruction files
9. Create `{IDEAS_FILE}` for deferred improvements
10. Measure: count instruction tokens per session before and after

### What to Customize
- Token budgets per layer and per role (calibrate to your model's context window)
- The specific content assigned to each layer
- Navigation map format (Q&A table, index, or directory convention)
- Role isolation mechanism (ignore file, separate directories, or prompt filtering)
- Ideas buffer format and review cadence

### What NOT to Change
- Four knowledge layers ordered by rate of change
- Downward-only migration direction (text -> config -> model -> code)
- Three instruction layers with progressive loading (always / first-task / on-failure)
- Mutual exclusivity of executor and developer roles
- Narrative before normative ordering in instruction files
- Anti-pattern: never duplicate Layer 3 content in Layer 1

<!-- HISTORY: load for audit -->
## Origin

- **Source agent:** spec-creator
- **Findings:** [15] Four-Layer Knowledge Architecture, [16] Executor/Developer Role Isolation, [22] Three-Layer Instruction Architecture, [57] Narrative Before Normative, [64] Ideas Buffer for Deferred Work, [74] Lazy Loading of Rules and Schemas
- **Discovered through:** Evolution of the spec-creator instruction system from a monolithic ~25K token file to a layered architecture. The four-layer model emerged from observing that different parts of the instructions changed at different rates: validators (Layer 0) hadn't changed in weeks, while batch strategies (Layer 2) changed every run. Role isolation was discovered after "knowledge leakage" incidents where the executor bypassed extraction rules based on understanding the designer's rationale. The three-layer instruction model reduced per-session context load by loading Layer 2-3 content on demand rather than upfront, saving ~10-15K tokens on schema loading alone.
- **Evidence:** Instruction reduction: 25K -> 8K tokens for executor sessions (-68%). Schema loading: from 10-15K upfront to 1.5-2K per schema on demand. Knowledge leakage incidents: eliminated after role isolation. Navigation: any question answerable in <=3 hops via nav map. Ideas preserved: 22 improvement ideas captured across 4 waves, zero lost.

## Related Patterns
- [Cognitive Offload](../methodologies/cognitive_offload.md) -- Layer 0 is the destination for offloaded mechanical work
- [Adversarial Reflection](../patterns/adversarial_reflection.md) -- developer-role activity, not executor-role
- [RAP Methodology](../methodologies/rap_methodology.md) -- the Algorithmization phase drives knowledge downward through layers
- [Context Budget](../methodologies/context_budget.md) -- layered loading is the primary mechanism for budget control
- [Maturity Metrics](../methodologies/maturity_metrics.md) -- kappa (Context Compression) measures Layer 0 vs. text ratio
- [Phase Gate](../patterns/phase_gate.md) -- gate documents are Layer 1 content, always loaded
- [Checkpoint Resume](../patterns/checkpoint_resume.md) -- session state is Layer 3 (instance), changes every run
- [Append-Only Audit](../patterns/append_only_audit.md) -- ideas buffer follows append-only mutation policy
- [Monitoring Principles](../best_practices/monitoring_principles.md) -- metrics/logs are developer-only context
