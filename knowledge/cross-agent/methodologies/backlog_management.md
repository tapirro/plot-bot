---
id: cross-backlog-management
title: Three-Tier Backlog Management
type: methodology
concern: [backlog-management]
mechanism: [taxonomy, registry]
scope: cross-cycle
lifecycle: [decide, act, reflect]
origin: harvest/voic-experiment
origin_findings: [17, 25, 48]
maturity: draft
domain: cross-agent/methodologies
confidence: observed
basis: "distilled from voic-experiment harvest, 65 findings from voice agent sessions"
---

# Three-Tier Backlog Management

<!-- CORE: load always -->
## Problem

Work items from multiple sources -- strategic roadmaps, quality system failures, research ideas -- compete for the same execution slots with no structural priority. Without separation, urgent system-generated fixes (a gate failure, a regression, a broken test) crowd out strategic roadmap work because they are immediate and concrete. The strategic plan stalls while the agent firefights an endless stream of auto-generated issues.

Exploratory work suffers even more. Research items and experimental ideas have uncertain payoff and no deadline, so they are perpetually deferred in favor of items with clear, measurable urgency. Over months, the agent becomes purely reactive -- competent at fixing what breaks but incapable of improving beyond its current capability envelope.

Without impact measurement, there is no feedback mechanism to detect when the agent is spending cycles on low-value work. A cycle of polish and minor fixes looks productive (items are completed, commits are made) but moves no key metric. The agent has no signal that it should shift composition toward higher-impact work.

## Solution

Three separated backlogs ensure that each category of work has dedicated capacity that cannot be cannibalized by the others. Tier 1 (Strategies) holds human-authored multi-phase roadmap items representing deliberate, planned progress. Tier 2 (Auto-backlog) holds system-generated items from quality failures, gate violations, and incident post-mortems. Tier 3 (Research) holds exploratory items with external references and hypotheses about potential improvements.

Each cycle selects exactly a fixed number of items with mandatory composition rules: minimum IMPACT items (from strategies and high-priority auto-backlog), maximum CRAFT items (from low-priority auto-backlog), and dedicated BOLD slots (from research). The composition rules guarantee that strategic work always progresses, maintenance has a bounded allocation, and exploration has a protected slot.

Impact scoring (1-5) after each cycle creates a quantitative feedback loop. Every completed item receives a score based on its measurable effect on key metrics. An adaptive composition rule uses the rolling average impact: when impact drops below a threshold, CRAFT slots are zeroed and reallocated to IMPACT, forcing the agent out of low-value polish cycles. When impact is consistently high, BOLD slots expand, rewarding high performance with more exploration.

The selection algorithm enforces priority: critical strategy phases are selected first (mandatory), remaining IMPACT slots are filled by priority score, CRAFT and BOLD slots are filled from their respective tiers. Empty slots in one tier are not backfilled from another, preserving the compositional intent. Completed items move to a done directory with their impact scores and outcomes recorded, enabling retrospective analysis of what types of work delivered the most value.

## Implementation

### Structure

```
BACKLOG
├── strategies/          ← Tier 1: human-authored roadmap items
│   ├── S-A.md           ← strategy A (multi-phase)
│   ├── S-B.md           ← strategy B
│   └── ...
├── auto/                ← Tier 2: system-generated from quality failures
│   ├── AB-001.md
│   ├── AB-002.md
│   └── ...
├── research/            ← Tier 3: exploration with external references
│   ├── R-01.md
│   └── ...
├── done/                ← completed items (all tiers)
│   └── ...
└── NEXT_CYCLE.md        ← selected items for upcoming cycle
```

### Tier Definitions

| Tier | Name | Source | ID Format | Lifecycle |
|------|------|--------|-----------|-----------|
| 1 — Strategies | Roadmap | Human-authored | `[S-{LETTER}.{PHASE}]` | Multi-phase, one phase = one cycle |
| 2 — Auto-backlog | System-generated | Quality failures, gate violations | `[AB-{NNN}]` | Single item, auto-rendered from quality state |
| 3 — Research | Exploration | External references, literature | `[R-{NN}:{REF_ID}]` | Single item, exploratory |

### Tier 1: Strategies

Human-authored multi-phase plans representing the roadmap. Each strategy is a document with numbered phases. One phase maps to one cycle of work.

```
STRATEGY
├── id: S-{LETTER}
├── title: string
├── author: "{OWNER_NAME}"
├── phases:
│   ├── S-{LETTER}.1: description, acceptance criteria
│   ├── S-{LETTER}.2: description, acceptance criteria
│   └── ...
├── current_phase: int
├── status: "active" | "paused" | "completed"
└── priority: "critical" | "high" | "normal"
```

Progress tracking: when phase N completes, `current_phase` increments. The next cycle selection picks `S-{LETTER}.{N+1}`.

### Tier 2: Auto-Backlog

System-generated items rendered from a quality state machine (e.g., validation gates, quality checks, incident post-mortems). Items appear automatically when quality thresholds are violated.

```
AUTO_ITEM
├── id: AB-{NNN}
├── title: string
├── source: "{QUALITY_SOURCE}"        ← which gate/check generated this
├── priority_score: float (0.0 - 1.0)
├── category: "IMPACT" | "CRAFT"
├── status: "open" | "in_progress" | "done" | "wont_fix"
├── created_at: datetime
└── evidence: string                   ← data that triggered creation
```

Priority score is computed from severity and frequency of the quality failure. Items are ranked by `priority_score` descending for selection.

### Tier 3: Research

Exploratory items referencing external knowledge sources (papers, articles, techniques). Each item links to a specific reference and describes what to explore and why.

```
RESEARCH_ITEM
├── id: R-{NN}:{REF_ID}
├── title: string
├── ref_url: string                    ← external reference link
├── hypothesis: string                 ← what we expect to learn/gain
├── status: "proposed" | "in_progress" | "done" | "abandoned"
└── outcome: string | null             ← what was learned (filled on completion)
```

Completed research items move to `done/` with their outcome documented.

### NEXT_CYCLE Selection

Each cycle selects exactly `{CYCLE_SIZE}` items with composition rules:

```
CYCLE_SELECTION
├── total_items: {CYCLE_SIZE}
├── min_impact: {MIN_IMPACT_ITEMS}     ← items categorized as high-impact
├── max_craft: {MAX_CRAFT_ITEMS}       ← items categorized as refinement
├── bold_slots: {BOLD_SLOTS}           ← research or experimental items
└── items: list of selected IDs
```

**Category classification:**

| Category | Description | Source Tiers |
|----------|-------------|-------------|
| IMPACT | Moves key metrics, addresses critical failures | Tier 1 (strategies), Tier 2 (high-priority auto) |
| CRAFT | Improves quality, reduces debt, polish | Tier 2 (low-priority auto) |
| BOLD | Exploratory, uncertain outcome, high potential | Tier 3 (research) |

**Selection algorithm:**

1. Pick all `critical` strategy phases first (mandatory, count toward IMPACT)
2. Fill remaining IMPACT slots from `high` strategies and top auto-backlog items by `priority_score`
3. Fill CRAFT slots from remaining auto-backlog items
4. Fill BOLD slots from research backlog
5. If not enough items in a tier, leave slots empty (do not backfill from another tier)

### Decision Rules

**Impact scoring** (post-cycle):

After each cycle, every completed item receives an impact score 1-5:

| Score | Meaning |
|-------|---------|
| 1 | Cosmetic — no measurable effect |
| 2 | Minor — small improvement, limited scope |
| 3 | Infrastructure — enables future work, no direct metric change |
| 4 | Significant — measurable improvement in key metric |
| 5 | Breakthrough — step-change in capability or performance |

**Adaptive composition rule:**

```
avg_impact = average(impact_scores from last {LOOKBACK_CYCLES} cycles)

IF avg_impact < {MIN_AVG_IMPACT}:
    next cycle: max_craft = 0, reallocate to IMPACT
    reason: "CRAFT items not delivering value, focusing on IMPACT"

IF avg_impact >= {HIGH_AVG_IMPACT}:
    next cycle: bold_slots += 1 (borrow from CRAFT)
    reason: "High performance, increasing exploration"
```

**Completion tracking:**

| Status | Rule |
|--------|------|
| Done | Item completed, impact scored, moved to `done/` |
| Carry-forward | Item started but not finished — carries to next cycle (counts toward total) |
| Wont-fix | Auto-backlog item determined unnecessary — closed with reason |
| Abandoned | Research item determined not worth pursuing — closed with outcome |

### Configuration

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `{CYCLE_SIZE}` | Total items per cycle | 4 | yes |
| `{MIN_IMPACT_ITEMS}` | Minimum IMPACT items per cycle | 2 | yes |
| `{MAX_CRAFT_ITEMS}` | Maximum CRAFT items per cycle | 1 | yes |
| `{BOLD_SLOTS}` | Research/experimental slots per cycle | 1 | yes |
| `{MIN_AVG_IMPACT}` | Impact threshold below which CRAFT is zeroed | 3.0 | yes |
| `{HIGH_AVG_IMPACT}` | Impact threshold above which BOLD expands | 4.0 | yes |
| `{LOOKBACK_CYCLES}` | Number of past cycles for impact averaging | 3 | yes |
| `{OWNER_NAME}` | Human owner who authors strategies | "team-lead" | yes |
| `{QUALITY_SOURCE}` | System that generates auto-backlog items | "validation-gates" | yes |

<!-- REFERENCE: load on adoption -->
## Adaptation Guide

### Prerequisites
- A recurring cycle structure (see Autonomous Loop Protocol)
- A quality/validation system that can generate items programmatically (for Tier 2)
- A human owner who maintains the strategic roadmap (for Tier 1)
- A file or database system for backlog persistence

### Steps to Adopt
1. Create the three-tier directory structure (strategies, auto, research, done)
2. Define your ID scheme following the `[S-X.N]`, `[AB-NNN]`, `[R-NN:ref]` convention
3. Author initial strategies as multi-phase documents in Tier 1
4. Connect your quality/validation system to auto-generate Tier 2 items
5. Seed Tier 3 with 2-3 research items worth exploring
6. Define your `{CYCLE_SIZE}` and composition rules (start with 4 = 2 IMPACT + 1 CRAFT + 1 BOLD)
7. Implement NEXT_CYCLE selection at the start of each cycle
8. Implement impact scoring (1-5) at the end of each cycle
9. Implement the adaptive composition rule (avg < threshold zeroes CRAFT)
10. Track completed items in `done/` with impact scores and outcomes

### What to Customize
- Cycle size and composition ratios (adjust to your throughput)
- Impact score definitions (calibrate to your domain)
- Adaptive thresholds (`{MIN_AVG_IMPACT}`, `{HIGH_AVG_IMPACT}`)
- Auto-backlog source and priority score formula
- Strategy phase granularity (how much work per phase)
- Research reference format (adapt to your domain's literature)

### What NOT to Change
- Three separate tiers — do not merge them into a single backlog
- Composition rules with minimums and maximums — prevents any tier from monopolizing cycles
- Impact scoring as a feedback loop — without it, composition never adapts
- CRAFT zeroing when impact is low — this is the mechanism that prevents polish-without-progress
- Strategy phases mapping to cycles — keeps strategic work progressing at a predictable pace
- Completed items tracked with outcomes — history enables retrospective analysis

<!-- HISTORY: load for audit -->
## Origin
- **Source agent:** voic-experiment
- **Findings:** [17] Three-Tier Backlog Structure, [25] NEXT_CYCLE Selection and Composition Rules, [48] Impact Scoring and Adaptive Composition
- **Discovered through:** 40+ cycles of iterative development. Early versions had a single flat backlog where urgent auto-generated fixes crowded out strategic work. Introducing three tiers with composition rules ensured strategies always progressed. The adaptive composition rule (zeroing CRAFT when avg impact < 3.0) was added after noticing cycles of low-value polish work. Impact scoring closed the loop: cycles that deliver low impact automatically shift focus to higher-impact work.
- **Evidence:** Strategy A completed 13 phases over 13 cycles with 67% improvement in key metric through systematic decomposition. CRAFT zeroing triggered twice, each time redirecting effort to higher-impact items.

## Related Patterns
- [Autonomous Loop Protocol](../patterns/loop_protocol.md) — provides the cycle structure within which backlog selection operates
- [Hypothesis-Driven Experimentation](../methodologies/experimentation.md) — each selected item benefits from hypothesis framing
- [Human Review Digest](../patterns/human_review_digest.md) — human feedback feeds back into impact scoring
- [Validation Gates](../patterns/validation_gates.md) — gate failures generate Tier 2 auto-backlog items
- [Closed-Loop Quality System](../methodologies/closed_loop_quality.md) — quality pipeline is the primary source of Tier 2 auto-backlog items
- [Observability Engine](../patterns/observability_engine.md) — observability gaps generate auto-backlog items
