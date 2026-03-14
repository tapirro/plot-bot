---
id: cross-cognitive-offload
title: Cognitive Offload
type: methodology
concern: [knowledge-architecture, context-management]
mechanism: [layered-architecture]
scope: system
lifecycle: [improve]
origin: harvest/spec-creator
origin_findings: [4, 21, 37, 39, 60, 67]
maturity: draft
domain: cross-agent/methodologies
confidence: observed
basis: "distilled from spec-creator harvest, 104 findings across 16+ agent sessions"
---

# Cognitive Offload

<!-- CORE: load always -->
## Problem

Agents spend 50-60% of effort on mechanical operations -- counting artifacts, indexing files, validating schemas, computing coverage metrics -- that consume context tokens and introduce probabilistic errors. An LLM asked to count 47 items will sometimes report 46 or 48; a script returns 47 every time.

Without systematic offloading, agent instructions bloat to accommodate step-by-step mechanical procedures. Each procedure eats context budget that could serve semantic work -- the actual value an LLM provides. The agent ends up spending most of its capacity on tasks a deterministic program handles better, faster, and more reliably.

The problem compounds over time: as the system matures, more mechanical operations accumulate, instruction files grow unboundedly, and the agent's effective capacity for judgment and creativity shrinks proportionally. What starts as a minor inefficiency becomes the dominant cost.

## Solution

Apply the **separation principle**: classify every operation in the agent workflow as either semantic (requires judgment, interpretation, creativity) or mechanical (describable as a finite automaton or formula). Mechanical operations go into scripts; semantic operations stay with the agent.

Enforce the boundary with three mechanisms:
1. **Script-First Rule** -- if a script exists for an operation, the agent MUST use it. Manual execution of scripted operations is forbidden.
2. **Rule of Three** -- any operation performed manually three times triggers mandatory script creation. The threshold is low enough to catch patterns early.
3. **"Script Proposes, Agent Corrects"** -- for operations at the semantic-mechanical boundary (batch planning, scaffold generation, draft preparation), the script performs algorithmic work and produces a proposal. The agent reviews, adjusts with domain judgment, and commits the result.

This separation reduced instruction tokens by 68% (25K to 8K), tool calls per batch by 70% (15-20 to 4-6), and eliminated probabilistic validation errors entirely in the originating system.

## Implementation

### Structure

```
OPERATION_CLASSIFICATION
|-- semantic (agent keeps)
|   |-- interpreting source code meaning
|   |-- determining business relevance
|   |-- writing natural language descriptions
|   |-- resolving ambiguities (UNK decisions)
|   +-- quality judgment on edge cases
+-- mechanical (goes to script)
    |-- counting artifacts
    |-- checking referential integrity
    |-- updating derived indexes
    |-- computing coverage metrics
    |-- validating schema conformance
    +-- generating file scaffolds

SCRIPT_TABLE
|-- operation: string
|-- script: string (path to tool)
|-- manual_allowed: false
+-- dry_run: true

COLLABORATION_MODES
|-- script_only
|   +-- agent calls script, uses output directly
|-- script_proposes_agent_corrects
|   |-- script generates draft/suggestion
|   +-- agent reviews, adjusts, commits
+-- agent_only
    +-- pure semantic work, no script involvement
```

### Script-First Rule

If a script exists for an operation, the agent **MUST** use it. Manual execution is forbidden.

| Operation | Script | Manual Allowed |
|-----------|--------|---------------|
| Count artifacts | `{TOOL_PREFIX} count` | no |
| Check referential integrity | `{TOOL_PREFIX} validate` | no |
| Update indexes | `{TOOL_PREFIX} sync-index` | no |
| Compute coverage | `{TOOL_PREFIX} coverage` | no |
| Validate schema conformance | `{TOOL_PREFIX} check-schema` | no |
| Generate file scaffolds | `{TOOL_PREFIX} scaffold` | no |
| Full checkpoint sequence | `{TOOL_PREFIX} checkpoint` | no |

### Script Design Requirements

All scripts MUST support:

| Capability | Description | Example |
|-----------|-------------|---------|
| `--dry-run` | Preview changes without writing | `{TOOL_PREFIX} sync-index --dry-run` |
| Idempotency | Safe to call repeatedly, same result | Re-running checkpoint is safe |
| JSON output | Structured result with `action_required` field | `{"ok": true, "action_required": "none"}` |
| Single entry point | One CLI with subcommands | `{TOOL_PREFIX} <subcommand>` |

### "Script Proposes, Agent Corrects" Model

For tasks at the semantic-mechanical boundary:

1. Script performs algorithmic work (clustering, planning, draft generation)
2. Script outputs a proposal (draft, plan, suggestions)
3. Agent evaluates proposal using domain understanding
4. Agent makes semantic corrections and commits result

This is the safe middle ground: the script guarantees structural correctness and reduces context burn; the agent contributes quality judgment.

### Quantified Impact

Measured before/after introducing cognitive offload in spec-creator:

| Metric | Before | After | Change |
|--------|--------|-------|--------|
| Instruction tokens per session | ~25K | ~8K | -68% |
| Tool calls per batch | 15-20 | 4-6 | -70% |
| Validation errors | Probabilistic | Deterministic | Zero misses |
| Mechanical work fraction | 50-60% | ~0% | Eliminated |
| Context freed for semantic work | -- | ~17K tokens | Reclaimed |

### Configuration

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `{TOOL_PREFIX}` | CLI entry point for the toolkit | `./tools/spec` | yes |
| `{SEMANTIC_OPS}` | List of operations reserved for the agent | interpret, describe, judge | yes |
| `{MECHANICAL_OPS}` | List of operations delegated to scripts | count, validate, index | yes |
| `{RULE_OF_THREE_THRESHOLD}` | Repetitions before mandatory scripting | 3 | yes |
| `{DRY_RUN_DEFAULT}` | Whether new scripts default to dry-run | true | no |

### Decision Rules

```
FOR each operation in agent workflow:

  1. Can it be described as a finite automaton or formula?
     -> YES -> implement as script, forbid manual execution
     -> NO  -> keep as agent task

  2. Does a script exist for this operation?
     -> YES -> agent MUST use it (Script-First Rule)
     -> NO  -> agent performs manually, but flag for future scripting

  3. Is the operation done 3+ times?
     -> YES -> STOP and create a script now (Rule of Three)

  4. Is the operation at the semantic-mechanical boundary?
     -> YES -> use "Script Proposes, Agent Corrects" model
     -> NO  -> pure script or pure agent

  5. Can operations be parallelized via wave-based execution?
     -> YES -> script handles batching, agent reviews results per wave
```

### Wave-Based Parallel Execution

Scripts enable parallel processing where agent would do sequential:

1. Script generates a wave plan (independent batches)
2. Agent reviews the plan (semantic check)
3. Script executes wave N (parallel within wave)
4. Mandatory checkpoint between waves
5. Agent reviews wave results, corrects if needed
6. Repeat until all waves complete

<!-- REFERENCE: load on adoption -->
## Adaptation Guide

### Prerequisites
- An agent workflow with identifiable mechanical operations
- A scripting language available in the agent environment (Python, Bash)
- A convention for script location and invocation

### Steps to Adopt
1. Audit current agent workflow: list all operations, classify as semantic or mechanical
2. Quantify mechanical effort (% of context tokens, % of tool calls)
3. Implement scripts for the top 3-5 mechanical operations
4. Create the Script Table mapping operations to scripts
5. Add Script-First Rule to agent instructions with the prohibition table
6. Add `--dry-run` and JSON output to all scripts
7. Measure before/after: instruction tokens, tool calls per task, error rate
8. Identify boundary operations for "Script Proposes, Agent Corrects"
9. Implement the Rule of Three: flag repeated manual operations for scripting

### What to Customize
- The specific operations in the Script Table (domain-dependent)
- The `{TOOL_PREFIX}` and CLI structure
- Which operations use "Script Proposes, Agent Corrects" vs. pure script
- The Rule of Three threshold (could be 2 or 5 depending on overhead)
- Wave size and parallelism limits

### What NOT to Change
- The separation principle (finite automaton -> script, semantic judgment -> agent)
- Script-First Rule enforcement (if script exists, agent MUST use it)
- `--dry-run` and idempotency requirements for all scripts
- JSON output with `action_required` field
- The prohibition on manual execution of scripted operations

<!-- HISTORY: load for audit -->
## Origin

- **Source agent:** spec-creator
- **Findings:** [4] Cognitive Offload Architecture, [21] Disk-First Rule for Sub-Agent Output, [37] Context Budget Accounting, [39] "Script Proposes, Agent Corrects" Pattern, [60] Script-First Rule, [67] Wave-Based Parallel Execution with Checkpoint Boundaries
- **Discovered through:** Systematic analysis of where spec-creator agents wasted effort. Measurement showed ~60% of pre-extraction, ~50% of each batch, and ~100% of coverage audit was mechanical work (scanning files, checking schemas, updating indexes, counting coverage). Eight scripts were built that reduced instructions from ~25K to ~8K tokens (-68%), tool calls per batch from 15-20 to 4-6 (-70%), and validation errors from probabilistic to deterministic (zero misses). The "Script Proposes, Agent Corrects" model emerged as the safe middle ground for boundary operations like batch planning and scaffold preparation.
- **Evidence:** Instruction token reduction: 25K -> 8K (-68%). Tool calls per batch: 15-20 -> 4-6 (-70%). Validation errors: probabilistic -> deterministic. Mechanical work fraction: 50-60% before, near-zero after scripting. Context budget freed: ~17K tokens redirected to semantic work.

## Related Patterns
- [Knowledge Layer Architecture](../methodologies/knowledge_layers.md) -- knowledge migration downward is the long-term expression of cognitive offload
- [Adversarial Reflection](../patterns/adversarial_reflection.md) -- semantic work that stays with the agent, not offloadable
- [Subagent Coordination](../patterns/subagent_coordination.md) -- scripts enable wave-based parallel subagent execution
- [Checkpoint Resume](../patterns/checkpoint_resume.md) -- checkpoint scripts are a key offload target
- [Validation Gates](../patterns/validation_gates.md) -- gate scripts replace manual validation
- [Autonomous Loop Protocol](../patterns/loop_protocol.md) -- loop efficiency depends on mechanical work being scripted
- [Context Budget](../methodologies/context_budget.md) -- quantifies the freed context from offloading
- [Maturity Metrics](../methodologies/maturity_metrics.md) -- delta (Process Determinism) and phi (Semantic Focus) measure offload progress
- [Invariant-Based Testing](../methodologies/invariant_testing.md) -- script-based invariant checks are cognitive offload in the testing domain
- [Analysis-Review-Merge Pipeline](../methodologies/analysis_review_merge.md) -- review gate is mechanical work offloaded to a script
