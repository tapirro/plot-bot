---
id: cross-rap-methodology
title: Repository-as-Product (RaP)
type: methodology
concern: [knowledge-architecture]
mechanism: [pipeline]
scope: system
lifecycle: [act, reflect, improve]
origin: harvest/spec-creator
origin_findings: [14, 29, 30, 43, 49]
maturity: draft
domain: cross-agent/methodologies
confidence: observed
basis: "distilled from spec-creator harvest, 104 findings across 16+ agent sessions"
---

# Repository-as-Product (RaP)

<!-- CORE: load always -->
## Problem

Agent systems are typically built as code that compiles into an application. But LLM agents do not compile -- they interpret instructions, scripts, and context at runtime. The repository is not source code for a binary; it is the product itself, consumed directly by the agent at execution time.

Treating the repository as traditional source code leads to a predictable failure mode: teams over-engineer infrastructure (CI pipelines, deployment scripts, monitoring dashboards) while under-investing in the instructions and domain knowledge that actually determine agent behavior. A perfectly architected codebase with vague or contradictory instructions produces poor results, while a simple repository with precise, well-structured instructions performs well.

Improvement cycles lack empirical grounding because session logs are treated as debugging artifacts and discarded after use. Without preserved logs, reflection is guesswork: teams cannot measure which operations consume the most tokens, which instructions cause confusion, or which patterns repeat across sessions. The same mistakes recur because there is no feedback loop from execution back to instruction design.

## Solution

Treat the repository itself as the product. Installation is `git clone`; the agent IDE is the runtime; instructions and domain knowledge are the primary deliverables, not the code that surrounds them.

Development follows a four-phase cycle: Formulate (write instructions and rules as text), Execute (run real tasks on real data), Reflect (analyze session logs to find mechanical patterns, failure modes, and context waste), and Algorithmize (migrate stable patterns from text into code -- scripts, validators, schemas).

Knowledge migrates strictly downward through four layers as it stabilizes: text instructions become configuration parameters, configuration becomes model definitions, and models become code. Upward migration -- code reverting to text -- is a deliberate rollback signal indicating the rule was premature or wrong. The metric kappa (code knowledge / total knowledge) tracks this compression over time.

Session logs are the primary asset for improvement, not a debugging afterthought. Every tool call, token count, and decision is recorded in structured JSONL format. Log analysis reveals which operations are mechanical (candidates for scripting), which consume excessive context (candidates for optimization), and which fail repeatedly (candidates for instruction revision).

Role isolation separates the executor (who runs tasks using instructions) from the developer (who improves the system using logs and metrics). Mixing roles in one session causes knowledge leakage: the executor begins reasoning about exceptions and bypassing rules, undermining the determinism that instructions are meant to provide.

## Implementation

### Structure

```
RAP_CYCLE
├── formulate
│   ├── input: domain knowledge, user requirements
│   ├── output: markdown instructions, rules, schemas
│   └── principle: "write what you know, mark what you don't (UNK)"
├── execute
│   ├── input: instructions + real data
│   ├── output: artifacts + session logs
│   └── principle: "real tasks on real data, not synthetic tests"
├── reflect
│   ├── input: session logs (JSONL), outputs, error patterns
│   ├── output: gaps, contradictions, redundancies, stable patterns
│   └── principle: "logs are primary asset, not outputs"
└── algorithmize
    ├── input: stable patterns from reflection
    ├── output: scripts, validators, generators
    └── principle: "if describable as finite automaton, it goes into code"

KNOWLEDGE_MIGRATION
├── direction: text → config → models → code (ONLY downward)
├── trigger: pattern stable across 3+ executions
├── upward_migration: deliberate rollback (rule was wrong)
└── metric: kappa (Context Compression) = code_knowledge / total_knowledge

REPOSITORY_STRUCTURE (as product)
├── instructions/     # Layer 3: instance config, per-run overrides
├── rules/            # Layer 2: strategy, heuristics, discovery patterns
├── models/           # Layer 1: domain model, schemas, relationships
├── scripts/          # Layer 0: infrastructure, validation, state mgmt
├── context/          # Runtime state: session, logs, findings, indexes
└── logs/             # Session JSONL: tool calls, timestamps, tokens
```

### Knowledge Layer Migration

| Layer | Content | Format | Change Rate | Migration Trigger |
|-------|---------|--------|-------------|-------------------|
| 3 (Instance) | Per-project config, thresholds, overrides | YAML/env | Every run | -- (top layer) |
| 2 (Strategy) | Discovery heuristics, extraction patterns, batch planning | Markdown/TOML | Often | Pattern stable across 3+ runs |
| 1 (Domain Model) | Artifact types, relationships, mandatory fields | Pydantic/JSON Schema | On domain expansion | Config validated by code |
| 0 (Infrastructure) | Validation, state management, index generation | Python/scripts | Rarely | -- (bottom layer) |

Migration direction is strictly downward. Upward migration (code back to text) signals the rule was wrong and needs rethinking at a higher abstraction level.

### Configuration

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `{LOG_FORMAT}` | Session log format | `JSONL` with tool calls, timestamps, token counts | yes |
| `{LOG_RETENTION}` | How long to keep session logs | `all` (primary asset, never delete) | yes |
| `{STABILITY_THRESHOLD}` | Executions before pattern migrates to code | `3` | recommended |
| `{INSTRUCTION_BUDGET}` | Target instruction token count | `8K` (after optimization) | recommended |
| `{KAPPA_TARGET}` | Target fraction of knowledge in code | `0.6-0.8` | recommended |
| `{EXECUTOR_CONTEXT}` | Token budget for executor role | `3-5K` | recommended |
| `{DEVELOPER_CONTEXT}` | Token budget for developer role | `5-8K` | recommended |

### Decision Rules

```
AFTER each execution session:
  1. Were there repeated mechanical failures?
     → YES → candidate for algorithmization (text → script)
     → NO → continue

  2. Did the agent re-implement something a script should handle?
     → YES → add to "forbidden manual operations" table
     → NO → continue

  3. Is there a pattern stable across 3+ sessions?
     → YES → migrate one layer down:
         text instruction → config parameter → model field → code
     → NO → keep at current layer; premature algorithmization
            locks in unvalidated assumptions

  4. Did instruction tokens exceed budget?
     → YES → identify candidates for script extraction
     → Measure: instructions_before - instructions_after = context freed

  5. Is a code-level rule being violated repeatedly?
     → YES → the rule may be wrong; consider upward migration
     → This is a deliberate rollback, not a failure
```

### RAP Cycle in Practice

The cycle is observable empirically, often completing in 1-3 sessions:

```
SESSION 1 (Formulate + Execute):
  - Write text protocol (e.g., "merge materialization files by reading JSON")
  - Agent executes on real data
  - Agent hits context overflow reading materialization into context

SESSION 2 (Reflect):
  - Analyze JSONL session log
  - Find root cause: 30K tokens from materialization + 50K from TaskOutput
  - Identify stable pattern: "disk write + summary return"

SESSION 3 (Algorithmize):
  - Create merge_materialization.py script
  - Update instructions: replace 2 pages of text with "run merge_materialization.py"
  - Result: ~80K tokens saved per pipeline transition
```

Each cycle reduces instructions and increases determinism. The metric: tool calls per batch dropped from 15-20 to 4-6 after algorithmization.

### Logs as Primary Asset

```
SESSION_LOG (JSONL)
├── entry_type: tool_call | decision | error | checkpoint
├── timestamp: datetime
├── tool: string (if tool_call)
├── tokens_consumed: int
├── input_summary: string
├── output_summary: string
└── provenance
    ├── instruction_hash: string (which version of instructions)
    ├── toolkit_version: string
    ├── model: string
    └── session_id: string

LOG_ANALYSIS_OUTPUT
├── mechanical_patterns: list[{pattern, frequency, candidate_for_script}]
├── failure_modes: list[{symptom, root_cause, fix}]
├── context_consumption: {phase: tokens}
├── efficiency_metrics
│   ├── artifact_density: float (artifacts per 2K tokens)
│   ├── batch_yield: float (artifacts per batch)
│   ├── context_overflow_rate: float
│   └── script_utilization: float (scripted vs manual operations)
└── recommendations: list[string]
```

Without logs, reflection is guesswork. With logs, every improvement is data-driven.

### Role Isolation (Executor vs Developer)

```
EXECUTOR (runs tasks):
  - Reads: instructions, rules, schemas, toolkit cheatsheet
  - Context: 3-5K tokens
  - Does NOT know why rules exist
  - Does NOT see dev docs, logs, retrospectives
  - Enforcement: .executorignore hides dev files

DEVELOPER (improves system):
  - Reads: methodology, metrics, logs, history, known issues
  - Context: 5-8K tokens
  - Full access to all layers
  - Produces: instruction changes, new scripts, retrospectives

RULE: Never mix roles in one session
  - "Knowledge leakage" occurs when executor understands designer intent
  - Executor starts bypassing rules based on reasoning about exceptions
```

<!-- REFERENCE: load on adoption -->
## Adaptation Guide

### Prerequisites
- An agent system with text-based instructions (markdown, YAML)
- Ability to capture session logs (tool calls, token counts)
- A scripting language for automation (Python, Bash)
- Version control (git) for tracking knowledge migration

### Steps to Adopt
1. Structure the repository as the product: instructions, rules, models, scripts, context, logs
2. Capture complete session logs (JSONL with tool calls, timestamps, token counts)
3. After 3 execution sessions, perform first reflection: analyze logs for mechanical patterns
4. Extract the most frequent mechanical pattern into a script (first algorithmization)
5. Measure before/after: instruction tokens, tool calls per operation, error rate
6. Establish a "forbidden manual operations" table as patterns are scripted
7. Track kappa (knowledge in code / total knowledge) as a maturity metric
8. Separate executor and developer roles with distinct instruction sets
9. Iterate: each RAP cycle reduces text and increases determinism

### What to Customize
- Layer definitions (adapt to your domain's knowledge structure)
- Log format and analysis tooling
- Stability threshold for migration (3 is a starting point)
- Token budgets per role
- Specific scripts and validators (domain-dependent)

### What NOT to Change
- The four-phase cycle order (Formulate -> Execute -> Reflect -> Algorithmize)
- Downward-only knowledge migration (text -> config -> models -> code)
- Logs as primary asset (never discard session logs)
- Role isolation between executor and developer
- The principle: start with text, not code. Premature algorithmization locks in unvalidated assumptions
- Real tasks on real data (not synthetic tests) for the Execute phase

<!-- HISTORY: load for audit -->
## Origin

- **Source agent:** spec-creator
- **Findings:** [14] Repository-as-Product Methodology (methods), [29] Logs as Primary Asset (methods), [30] Four-Layer Knowledge Architecture (methods), [43] RAP Cycle Text->Problem->Script (lessons), [49] Cognitive Offload Architecture (lessons)
- **Discovery story:** The spec-creator system evolved through observable RAP cycles. The first major cycle (3 commits over 1 day): text protocol for disk-first materialization was written, agent executed and hit context overflow (30K + 50K tokens), JSONL log forensics identified the root cause (not "too many agents" as the user assumed, but reading files into context), and `merge_materialization.py` was created. The cognitive offload analysis formalized this pattern: systematic measurement showed 60% of pre-extraction, 50% of each batch, and 100% of coverage audit was mechanical work. Eight scripts reduced instructions from 25K to 8K tokens (68% reduction) and tool calls from 15-20 to 4-6 per batch. The methodology itself was documented after observing this cycle repeat across multiple system features.
- **Evidence:** Instructions: 25K -> 8K tokens (68% reduction). Tool calls per batch: 15-20 -> 4-6 (70% reduction). Validation errors: probabilistic -> deterministic (0 misses). RAP cycle observed in merge materialization (1 day, 3 commits), review gate (2 incidents -> script), and sub-agent contract evolution (v1 through v4.2, each driven by log forensics). Sub-agent contract evolved through 6 versions, each triggered by production log analysis.

## Related Patterns
- [Context Budget Management](../methodologies/context_budget.md) -- algorithmization directly reduces instruction token cost
- [Append-Only Audit Trail](../patterns/append_only_audit.md) -- logs as primary asset requires append-only preservation
- [Closed-Loop Quality](../methodologies/closed_loop_quality.md) -- reflect phase feeds quality improvement loop
- [Validation Gates](../patterns/validation_gates.md) -- gate scripts are Layer 0 algorithmization of text rules
- [Loop Protocol](../patterns/loop_protocol.md) -- bounded loops are scripted from text protocols
- [Monitoring Principles](../best_practices/monitoring_principles.md) -- efficiency metrics from reflection phase
- Cognitive Offload (cross-cognitive-offload) -- the "semantics to agent, mechanics to code" boundary
- Knowledge Layers (cross-knowledge-layers) -- four-layer architecture detail
- Adversarial Reflection (cross-adversarial-reflection) -- split reflection into analyst/reviewer roles
- Maturity Metrics (cross-maturity-metrics) -- kappa, delta, iota, phi, sigma measurement system
- Analysis-Review-Merge (cross-analysis-review-merge) -- pipeline phase separation as RaP product
- [Knowledge Layer Architecture](../methodologies/knowledge_layers.md) -- four-layer knowledge architecture is the structural foundation of RaP
