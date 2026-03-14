---
id: cross-experimentation
title: Hypothesis-Driven Experimentation
type: methodology
concern: [experimentation]
mechanism: [pipeline]
scope: per-cycle
lifecycle: [decide, act, reflect]
origin: harvest/voic-experiment
origin_findings: [21, 23, 33, 34, 44]
maturity: draft
domain: cross-agent/methodologies
confidence: observed
basis: "distilled from voic-experiment harvest, 65 findings from voice agent sessions"
---

# Hypothesis-Driven Experimentation

<!-- CORE: load always -->
## Problem

Without structured hypotheses, agents cannot distinguish correlation from causation. A change is made, a metric improves, and the agent concludes the change caused the improvement -- but the real cause may be unrelated (different input data, time-of-day effects, upstream changes). False attribution leads to optimizing the wrong things and building on phantom improvements.

Agents naturally focus on visible, easily measured components while unmeasured overhead silently dominates. In one documented case, 72% of total processing time was in unmeasured "Other" while the agent spent cycles optimizing the visible 28%. Without systematic decomposition and measurement, effort flows to the most salient problem rather than the largest one.

Failed experiments are particularly costly when undocumented. Without a record of what was tried and why it failed, the same dead-end hypotheses get re-explored by the same or different agents. Knowledge about what does not work is as valuable as knowledge about what does, but only if it is preserved.

## Solution

Each development cycle begins with a one-line falsifiable hypothesis and a measurable target. The hypothesis follows a strict format: "Changing {what} in {where} will {expected effect} by {measurable amount}." This format prevents vague goals ("make it faster") and ensures every experiment has a clear success criterion.

Measurements use percentiles (P50, P90, P99) over a minimum number of iterations, never single-run conclusions. Before/after comparison tables with delta and delta-percentage provide the evidence for verification. At cycle end, the hypothesis is marked confirmed, partial, or rejected -- and rejected hypotheses are documented with equal rigor, including what actually happened and what to try instead.

Before optimizing any component, a waterfall top-offender analysis decomposes the pipeline into measured segments, ranks them by contribution, and flags unstable segments (where P90/P50 ratio exceeds a threshold). The "Other" rule is enforced: if unmeasured overhead exceeds a configurable threshold, the agent must instrument it before optimizing anything else. This rule exists because the real bottleneck is often hidden in the unmeasured portion.

Impact scoring (1-5) on each cycle's work closes the loop between experimentation results and backlog prioritization, ensuring that high-impact strategies receive continued investment while low-impact work is deprioritized.

## Implementation

### Structure

```
CYCLE_RECORD
├── cycle_id: int
├── hypothesis: string                 ← 1 line, falsifiable
├── target: string                     ← measurable outcome
├── method: string                     ← what will be changed
├── measurements:
│   ├── before: {METRIC_TABLE}         ← baseline data
│   └── after: {METRIC_TABLE}          ← result data
├── verification:
│   ├── result: "confirmed" | "rejected" | "partial"
│   ├── evidence: string               ← data summary
│   └── lesson: string                 ← what was learned (especially on rejection)
└── related_items: list of backlog IDs ← what was worked on
```

### Hypothesis Format

A valid hypothesis is:

```
"Changing {WHAT} in {WHERE} will {EXPECTED_EFFECT} by {MEASURABLE_AMOUNT}"
```

Examples of valid hypotheses:
- "Batching {PIPELINE_STAGE} calls from 1-at-a-time to groups of {BATCH_SIZE} will reduce total processing time by {PERCENT}%"
- "Caching {RESOURCE} lookups will reduce {METRIC_NAME} P50 from {BEFORE} to {AFTER}"
- "Decomposing {COMPONENT} into {N} sub-stages will reveal where the majority of latency resides"

Examples of invalid hypotheses:
- "Make it faster" (not measurable)
- "Try a different approach" (not falsifiable)
- "Improve quality" (no target)

### Measurement Protocol

**Minimum iterations:** `{MIN_ITERATIONS}` per measurement point. Never draw conclusions from a single run.

**Metric table format:**

| Metric | P50 | P90 | P99 | Unit | Iterations |
|--------|-----|-----|-----|------|------------|
| `{METRIC_NAME}` | value | value | value | `{UNIT}` | `{MIN_ITERATIONS}` |

**Before/after comparison:**

| Metric | Before (P50) | After (P50) | Delta | Delta % |
|--------|-------------|------------|-------|---------|
| `{METRIC_NAME}` | value | value | value | value |

### Waterfall Top-Offender Analysis

A systematic method for finding where time, resources, or errors are actually spent. Used before optimizing anything.

**Steps:**

1. **Decompose:** Break the pipeline into measured segments (see Observability Engine)
2. **Rank:** Sort segments by contribution (time, error count, resource usage) descending
3. **Compare:** Run `{MIN_ITERATIONS}` iterations, compute percentiles for each segment
4. **Flag unstable:** If a segment's P90/P50 ratio exceeds `{INSTABILITY_THRESHOLD}`, flag it as unstable (inconsistent performance is often more important than slow-but-stable)
5. **Identify "Other":** Calculate unmeasured overhead as `total - sum(measured_segments)`
6. **Act on top offender:** Optimize the largest contributor first

**The "Other" Rule:**

```
other_pct = (total - sum(measured)) / total × 100

IF other_pct > {OTHER_THRESHOLD}%:
    DO NOT optimize measured segments
    FIRST: instrument "Other" to decompose it
    REASON: the real bottleneck is hidden in unmeasured overhead
```

This rule exists because of a documented case where "Other" was 72% of total time. Optimizing the visible 28% would have yielded marginal gains. Instrumenting the unmeasured portion revealed the actual bottleneck.

### Verification Protocol

At cycle end, verify the hypothesis:

| Result | Criteria | Action |
|--------|----------|--------|
| Confirmed | Target met or exceeded, data supports causal claim | Document success, update baseline |
| Partial | Direction correct but target not met | Document partial result, carry forward or revise hypothesis |
| Rejected | No improvement or regression | Document failure with equal rigor, extract lesson |

**Rejection documentation is mandatory.** A rejected hypothesis with a documented lesson is more valuable than an undocumented success, because it prevents the same dead end from being explored again.

### Impact Scoring

Each cycle's work receives an impact score:

| Score | Meaning | Example |
|-------|---------|---------|
| 1 | Cosmetic | Renamed a variable, reformatted output |
| 2 | Minor | Small improvement, limited scope, < `{MINOR_THRESHOLD}` improvement |
| 3 | Infrastructure | Enables future work, added measurement points, no direct metric change |
| 4 | Significant | `{SIGNIFICANT_THRESHOLD}` or more improvement in key metric |
| 5 | Breakthrough | Step-change in capability, opened new optimization surface |

### Multi-Phase Strategy Tracking

Long-running strategies span multiple cycles. Track cumulative progress:

```
STRATEGY_PROGRESS
├── strategy_id: S-{LETTER}
├── total_phases: int
├── completed_phases: int
├── cumulative_improvement: string     ← e.g., "67% reduction in {METRIC_NAME}"
├── phases:
│   ├── phase 1: hypothesis, result, impact_score
│   ├── phase 2: hypothesis, result, impact_score
│   └── ...
└── retrospective: string             ← overall lessons after completion
```

### Decision Rules

**No speculation without evidence:**
- Every claim of improvement MUST cite before/after data
- Every claim of cause MUST have controlled measurement (change one thing at a time)
- Without measurement, an improvement claim has no basis

**Instrument before optimizing:**
- If "Other" exceeds `{OTHER_THRESHOLD}%`, the agent adds measurement points first
- If a segment is unstable (P90/P50 > `{INSTABILITY_THRESHOLD}`), the agent investigates variance before optimizing
- If baseline data has fewer than `{MIN_ITERATIONS}` iterations, the agent collects more data first

**Failed hypothesis protocol:**
- Record the hypothesis exactly as stated
- Record what actually happened (data)
- Record why it failed (analysis)
- Record what to try instead (next step)
- Do NOT delete or hide failed hypotheses

### Configuration

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `{MIN_ITERATIONS}` | Minimum measurement iterations | 3 | yes |
| `{OTHER_THRESHOLD}` | % of "Other" that triggers instrumentation | 30 | yes |
| `{INSTABILITY_THRESHOLD}` | P90/P50 ratio flagging unstable segments | 2.0 | yes |
| `{MINOR_THRESHOLD}` | Improvement % below which score = 2 | 10% | yes |
| `{SIGNIFICANT_THRESHOLD}` | Improvement % at which score = 4 | 25% | yes |
| `{METRIC_NAME}` | Primary metric being optimized | "processing_time" | yes |
| `{UNIT}` | Unit of measurement | "ms" | yes |
| `{BATCH_SIZE}` | Default batch size for batching experiments | 50 | no |
| `{PIPELINE_STAGE}` | Name of pipeline stage being measured | "analysis" | context-dependent |

<!-- REFERENCE: load on adoption -->
## Adaptation Guide

### Prerequisites
- A recurring cycle structure where hypotheses can be formed and verified (see Autonomous Loop Protocol)
- An observability system that can provide before/after measurements (see Observability Engine)
- A backlog system to track what was worked on per cycle (see Three-Tier Backlog Management)
- A persistence mechanism for cycle records

### Steps to Adopt
1. Define your primary metric (`{METRIC_NAME}`) and unit of measurement
2. Establish a baseline: run `{MIN_ITERATIONS}` iterations and record P50/P90/P99
3. For the next cycle, write a 1-line hypothesis with a measurable target
4. Implement the change described in the hypothesis
5. Measure: run `{MIN_ITERATIONS}` iterations post-change, record P50/P90/P99
6. Compare before/after using the metric table format
7. Verify: mark as confirmed/partial/rejected with evidence
8. Score impact 1-5
9. If rejected, document the lesson with equal rigor as a success
10. Before optimizing any component, run waterfall top-offender analysis to confirm it is actually the top offender
11. If "Other" exceeds `{OTHER_THRESHOLD}%`, instrument it before optimizing anything else

### What to Customize
- Primary metric and unit (adapt to your domain)
- Impact score thresholds (calibrate to your scale of improvements)
- Minimum iterations (increase for noisy measurements, decrease for deterministic ones)
- "Other" threshold (lower for precision-critical domains, higher for rapid prototyping)
- Instability threshold (adjust based on acceptable variance in your domain)
- Hypothesis format (add domain-specific fields if needed)

### What NOT to Change
- 1-line hypothesis per cycle — prevents scope creep and ensures falsifiability
- Before/after measurement with percentiles — means and averages hide outliers
- Minimum iteration count — single-run conclusions are unreliable
- Rejected hypothesis documentation — deleting failures means repeating them
- "Other" rule — optimizing visible segments while ignoring unmeasured overhead is the most common experimentation mistake
- Impact scoring — without it, there is no feedback loop to the backlog
- No speculation without evidence — every claim must cite data

<!-- HISTORY: load for audit -->
## Origin
- **Source agent:** voic-experiment
- **Findings:** [21] Hypothesis-Driven Cycles, [23] Waterfall Top-Offender Analysis, [33] "Other" Decomposition Breakthrough, [34] Multi-Phase Strategy Tracking, [44] Impact Scoring Integration
- **Discovered through:** 40+ cycles of pipeline optimization. The hypothesis discipline emerged early as a way to prevent aimless tinkering. The waterfall top-offender method was formalized after noticing agents optimizing visible-but-minor components. The "Other" decomposition breakthrough occurred when 72% of total time was in unmeasured overhead — instrumenting it revealed the actual bottleneck and led to a 67% latency reduction over 13 systematic phases. Impact scoring was added to close the loop between experimentation results and backlog prioritization.
- **Evidence:** Strategy A: 13 cycles, 67% cumulative latency reduction. The "Other" instrumentation alone redirected optimization effort from a 5% contributor to the actual 72% contributor. Failed hypotheses documented across cycles prevented at least 3 known dead-end re-explorations.

## Related Patterns
- [Three-Tier Backlog Management](../methodologies/backlog_management.md) — impact scores feed into backlog adaptive composition
- [Observability Engine](../patterns/observability_engine.md) — provides the measurement infrastructure for before/after comparison
- [Human Review Digest](../patterns/human_review_digest.md) — human feedback complements automated measurement
- [Autonomous Loop Protocol](../patterns/loop_protocol.md) — provides the cycle structure for hypothesis-verify rhythm
- [Monitoring Principles](../best_practices/monitoring_principles.md) — "Instrument Before Optimizing" and "Hypothesis-Driven" principles
