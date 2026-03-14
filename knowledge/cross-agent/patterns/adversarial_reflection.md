---
id: cross-adversarial-reflection
title: Adversarial Reflection
type: pattern
concern: [testing, confidence-scoring]
mechanism: [scoring-model]
scope: per-cycle
lifecycle: [reflect]
origin: harvest/spec-creator
origin_findings: [17]
maturity: draft
domain: cross-agent/patterns
confidence: observed
basis: "distilled from spec-creator harvest, 104 findings across 16+ agent sessions"
---

# Adversarial Reflection

<!-- CORE: load always -->
## Problem

Self-analysis by a single agent converges to local optima. The agent has the same blind spots during reflection as it had during execution -- it cannot catch errors it is systematically predisposed to make. Asking an agent to "review your own work carefully" does not change its underlying biases; it merely re-runs the same reasoning with a slightly different prompt.

A single-pass review gravitates toward one of two failure modes. In the generous mode, the agent over-accepts: high recall but low precision, letting unsupported claims and hallucinated details through. In the strict mode, the agent over-rejects: high precision but low recall, missing gaps and incomplete coverage. A prompt asking for "balanced" review produces a comfortable middle ground that is neither complete nor precise -- the agent splits the difference rather than genuinely challenging its own output.

The fundamental issue is that quality review requires tension between competing objectives, and a single agent optimizing a single objective cannot generate that tension internally.

## Solution

Split reflection into two separate LLM calls with **opposing objectives**, creating productive tension that approximates a better optimum than either role achieves alone:

1. **Analyst pass** -- system prompt optimized for completeness. Asks: "What did we miss? What gaps exist? What is not covered that should be?" Output: a list of gaps, suggestions, and potential additions. The Analyst's risk is hallucination and over-inclusion.

2. **Reviewer pass** -- system prompt optimized for precision. Receives the original work product plus the Analyst's output. Asks: "Is each item supported by evidence? What should be removed as unsupported?" Output: items classified as confirmed, rejected, or uncertain. The Reviewer's risk is under-coverage and missed items.

3. **Resolution** -- structured merge where items confirmed by both roles are accepted, items rejected by the Reviewer are dropped (or flagged for human review), and uncertain items are kept with low confidence scores.

The effect is amplified by using different models or providers for each role, since different models have different systematic errors. A hallucination pattern in one model is unlikely to be shared by another, making cross-model review more effective at catching false positives. Maximum reflection rounds are bounded to prevent infinite loops, with unresolved conflicts escalated to a human or orchestrator.

## Implementation

### Structure

```
REFLECTION_PAIR
|-- analyst
|   |-- objective: maximize_recall (completeness)
|   |-- risk: hallucination, over-inclusion
|   |-- prompt_focus: "What's missing? What's not covered?"
|   +-- output: list of gaps, suggestions, potential additions
|-- reviewer
|   |-- objective: maximize_precision (correctness)
|   |-- risk: under-coverage, missed items
|   |-- prompt_focus: "Is this supported by evidence? Remove unsupported."
|   +-- output: list of issues, removals, confidence downgrades
+-- resolution
    |-- method: structured merge of both outputs
    |-- conflicts: escalate to orchestrator or human
    +-- metric: F1-like balance of precision and recall
```

### Reflection Cycle

```
INPUT (work product to reflect on)
  |
  v
ANALYST PASS (separate LLM call)
  System prompt: completeness-oriented
  Question: "What did we miss? What gaps exist?
             What's not covered that should be?"
  Output: gaps[], suggestions[], potential_additions[]
  |
  v
REVIEWER PASS (separate LLM call, optionally different model)
  System prompt: precision-oriented
  Input: original work product + Analyst's output
  Question: "Is each item supported by evidence?
             What should be removed as unsupported?"
  Output: confirmed[], rejected[], uncertain[]
  |
  v
RESOLUTION (merge)
  Analyst found + Reviewer confirmed   -> ACCEPT
  Analyst found + Reviewer rejected    -> DROP (or flag for human)
  Analyst found + Reviewer uncertain   -> KEEP with low confidence
  |
  v
OUTPUT (refined work product with confidence annotations)
```

### Opposing Objective Pairs

The Analyst/Reviewer split generalizes to any pair of opposing objectives:

| Analyst Role | Reviewer Role | Domain |
|-------------|--------------|--------|
| Completeness (minimize misses) | Precision (minimize false positives) | Quality review |
| Breadth (explore widely) | Depth (validate thoroughly) | Research |
| Speed (ship fast) | Safety (catch risks) | Deployment |
| Exploration (try new approaches) | Exploitation (use proven methods) | Strategy |
| Generosity (include edge cases) | Parsimony (keep only clear cases) | Classification |

### Cross-Model Amplification

Using different models for Analyst and Reviewer amplifies the adversarial effect:

| Configuration | Effect | When to Use |
|--------------|--------|-------------|
| Same model, same provider | Baseline adversarial tension | Default, lowest cost |
| Same model, different temperature | Moderate amplification | When cost is a concern |
| Different model, same provider | Good amplification | When API access allows |
| Different model, different provider | Maximum amplification | High-stakes reflection |

Different models have different systematic errors. A hallucination pattern in Model A is unlikely to be shared by Model B, making cross-model review more effective at catching false positives.

### Configuration

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `{ANALYST_MODEL}` | Model for the Analyst pass | gpt-4o | yes |
| `{REVIEWER_MODEL}` | Model for the Reviewer pass | claude-sonnet | no |
| `{ANALYST_PROMPT}` | System prompt emphasizing completeness | "Find all gaps..." | yes |
| `{REVIEWER_PROMPT}` | System prompt emphasizing precision | "Verify evidence..." | yes |
| `{CONFIDENCE_THRESHOLD}` | Below this, Reviewer-uncertain items are dropped | 0.3 | no |
| `{MAX_REFLECTION_ROUNDS}` | Maximum Analyst-Reviewer cycles before forcing resolution | 2 | yes |
| `{ESCALATION_TARGET}` | Where unresolved conflicts go | human / orchestrator | yes |

### Decision Rules

```
FOR each reflection cycle:

  1. Run Analyst with completeness prompt
     -> output: gaps[], suggestions[]

  2. Run Reviewer with precision prompt on Analyst's output
     -> output: confirmed[], rejected[], uncertain[]

  3. Merge:
     - Analyst found + Reviewer confirmed   -> ACCEPT
     - Analyst found + Reviewer rejected     -> DROP (or flag for human)
     - Analyst found + Reviewer uncertain    -> KEEP with low confidence

  4. Are there unresolved conflicts (rejected items the Analyst insists on)?
     -> YES, round < {MAX_REFLECTION_ROUNDS} -> run another round
     -> YES, round >= {MAX_REFLECTION_ROUNDS} -> escalate to {ESCALATION_TARGET}
     -> NO -> finalize output

  5. Optional: use different model/provider for Reviewer
     -> amplifies effect (different systematic errors)

  6. Is the work product high-stakes (production deployment, external delivery)?
     -> YES -> use cross-model amplification (different provider for Reviewer)
     -> NO  -> same model with different prompts is sufficient
```

### Output Format

Each item in the reflection output carries a confidence annotation:

```
REFLECTION_ITEM
|-- id: string
|-- source: "analyst" | "reviewer" | "both"
|-- status: "accepted" | "rejected" | "uncertain" | "escalated"
|-- analyst_reasoning: string
|-- reviewer_reasoning: string (if reviewed)
+-- confidence: float (0.0 - 1.0)
    |-- accepted by both: >= 0.8
    |-- accepted by analyst, uncertain by reviewer: 0.4 - 0.6
    +-- escalated: null (pending human decision)
```

<!-- REFERENCE: load on adoption -->
## Adaptation Guide

### Prerequisites
- An agent workflow with a reflection or review step
- Ability to make separate LLM calls with different system prompts
- A merge/resolution mechanism (can be simple rule-based or human-in-the-loop)

### Steps to Adopt
1. Identify the reflection point in your workflow (where the agent reviews its own output)
2. Write two system prompts: one emphasizing completeness, one emphasizing precision
3. Replace the single reflection call with two sequential calls (Analyst then Reviewer)
4. Implement the merge logic (accept/reject/uncertain classification)
5. Define the escalation path for unresolved conflicts
6. Set `{MAX_REFLECTION_ROUNDS}` (start with 1, increase if quality warrants)
7. Measure: compare single-pass vs. adversarial reflection on precision and recall
8. Optional: experiment with cross-model amplification for high-stakes work

### What to Customize
- The specific opposing objectives (completeness/precision is the default, but others apply)
- System prompts for each role (domain-specific focus questions)
- Merge resolution rules (strictness of the Reviewer veto)
- Confidence thresholds for uncertain items
- Whether to use cross-model amplification (cost/benefit tradeoff)
- Maximum reflection rounds
- Escalation target (human, orchestrator, or higher-priority queue)

### What NOT to Change
- Two separate LLM calls with different system prompts (not a single "be balanced" prompt)
- Opposing objectives (both roles optimizing for the same thing defeats the purpose)
- Structured merge with explicit accept/reject/uncertain classification
- Escalation path for unresolved conflicts (never silently drop disputed items)
- Maximum round limit (unbounded reflection can loop indefinitely)

<!-- HISTORY: load for audit -->
## Origin

- **Source agent:** spec-creator
- **Findings:** [17] Adversarial Reflection (Analyst vs Reviewer split)
- **Discovered through:** The spec-creator pipeline separated analysis and review into distinct phases with different sub-agents. The Analyst sub-agents optimized for finding all artifacts (completeness), while Reviewer sub-agents verified findings against original sources (precision). When both roles were combined in a single agent pass, the agent converged on a comfortable middle ground that was neither complete nor precise. Splitting into two passes with opposing prompts consistently produced better results than a single "balanced" pass. The effect was further amplified when different models were used: errors that one model systematically made were caught by the other.
- **Evidence:** Single-pass review missed gaps that the completeness-focused Analyst caught, while also accepting unsupported claims that the precision-focused Reviewer rejected. The reading protocol for Reviewers (finding [11] in lessons) reduced overhead from ~25% to near zero, showing that focused sub-roles are more efficient than broad ones. The separation into analysis/review/merge phases (finding [152] in domain) formalized this adversarial structure at the pipeline level.

## Related Patterns
- [Cognitive Offload](../methodologies/cognitive_offload.md) -- adversarial reflection is semantic work that stays with the agent
- [Knowledge Layer Architecture](../methodologies/knowledge_layers.md) -- reflection is a developer-role activity, not executor-role
- [Confidence & Severity Model](../methodologies/confidence_severity.md) -- confidence scores from adversarial reflection feed into severity-gated decisions
- [Closed-Loop Quality System](../methodologies/closed_loop_quality.md) -- adversarial reflection is one mechanism within the quality feedback loop
- [Validation Gates](../patterns/validation_gates.md) -- gates can incorporate adversarial reflection for high-stakes transitions
- [Subagent Coordination](../patterns/subagent_coordination.md) -- Analyst and Reviewer can be implemented as sub-agents
- [Analysis-Review-Merge Pipeline](../methodologies/analysis_review_merge.md) -- pipeline-level expression of the adversarial split
- [Provider Resilience](../patterns/provider_resilience.md) -- cross-model amplification benefits from provider fallback
- [Scenario-First Testing](../methodologies/scenario_first_testing.md) -- adversarial reflection can validate scenario completeness
- [Maturity Metrics](../methodologies/maturity_metrics.md) -- iota (Invariant Density) improves as adversarial reflection catches more edge cases
