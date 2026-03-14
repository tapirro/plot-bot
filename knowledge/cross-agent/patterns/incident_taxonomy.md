---
id: cross-incident-taxonomy
title: Incident Taxonomy
type: pattern
concern: [anomaly-classification]
mechanism: [taxonomy]
scope: per-item
lifecycle: [classify]
origin: harvest/multi
origin_findings:
  hilart-ops-bot: [2]
  voic-experiment: [60, 49]
maturity: draft
domain: cross-agent/patterns
confidence: observed
basis: "cross-agent pattern from hilart-ops-bot and voic-experiment harvests"
---

# Incident Taxonomy

<!-- CORE: load always -->
## Problem

Free-text anomaly descriptions prevent aggregation, deduplication, and trend tracking. When incidents are described in prose ("the response was wrong," "processing took too long," "data was missing"), there is no reliable way to group related incidents, count recurrences, or compare severity across domains.

At scale, raw counts mislead. A frequent but trivial formatting error generates more noise than a rare but severe data integrity failure. Without a severity-aware priority model, agents process incidents in discovery order or by raw frequency, wasting effort on low-impact issues while critical failures go unaddressed.

The taxonomy itself presents a design challenge: too many categories upfront creates overhead and unused types, while too few forces dissimilar incidents into the same bucket, destroying analytical value. Categories must emerge from observed data, not speculative design, yet uncontrolled growth leads to overlapping types and inconsistent classification.

## Solution

A hierarchical incident coding system with the naming convention `{UNIT}-{CATEGORY}-{DETAIL}` provides structured, machine-readable classification. Each incident type carries a default severity, a metric class (instant, same-day, lagging), and a north star multiplier that weights its importance relative to the agent's primary objectives.

Severity modifiers are orthogonal to the type definition: scope, duration, compound occurrence, and recurrence each adjust severity independently. This separates "what happened" from "how bad is it," enabling the same incident type to carry different severities depending on context.

A priority scoring formula combines severity, capped frequency, recency, and north star alignment into a single comparable score. Capping frequency prevents high-volume trivial incidents from dominating the priority queue. Recency multipliers ensure recently observed issues are addressed before stale ones.

Deduplication uses composite keys (category + scenario ID + context range) to prevent the same root cause from appearing as multiple backlog items. Correlation detection flags temporally co-occurring incidents from different categories, surfacing shared root causes that individual category tracking would miss.

The taxonomy grows through a controlled evolution protocol: new types require three or more real observations, a formal proposal, and owner approval before being added. An "unknown" category acts as a pressure valve, collecting unclassifiable incidents until they accumulate enough evidence to justify a new type.

## Implementation

### Structure

```
INCIDENT_TYPE
├── code: string          # {UNIT}-{CATEGORY}-{DETAIL}
├── unit: string          # Which domain
├── category: string      # Broad class within domain
├── detail: string        # Specific variant
├── description: string   # Human-readable explanation
├── default_severity: enum
├── metric_class: INSTANT | SAME_DAY | LAGGING
└── north_star_multiplier: float  # weight relative to north star metrics
```

### Naming Convention

```
{UNIT_PREFIX}-{CATEGORY}-{DETAIL}
```

Examples of the pattern (not these specific types):
- `{UNIT_A}-PERF-DROP` -- performance drop in Unit A
- `{UNIT_B}-QUEUE-OVERFLOW` -- queue overflow in Unit B
- `CROSS-CASCADE-{UNIT_A}_{UNIT_B}` -- cross-unit cascading effect
- `DATA-PIPELINE-{SOURCE}` -- data pipeline issue

### Categories

Every agent should have at minimum:

| Category | Covers | Examples |
|----------|--------|---------|
| Per-unit types | Domain-specific anomalies | Performance drops, capacity issues, quality degradation |
| `CROSS-*` | Inter-unit correlations | Cascading failures, regional impacts |
| `DATA-*` | Data infrastructure | Pipeline outage, stale data, data gaps, interpretation errors |

### Severity Modifiers (Orthogonal)

Base severity comes from threshold deviation. Modifiers adjust +/-1 level:

| Modifier | Condition | Effect |
|----------|-----------|--------|
| Scope | Affects `{SCOPE_ALL}` regions/units | +1 |
| Duration | Persists > `{DURATION_THRESHOLD}` | +1 |
| Compound | 2+ simultaneous incidents | +1 |
| Recurring | 3+ in `{RECURRENCE_WINDOW}` | +1 |
| Isolated | Single region, single metric | -1 |

### Priority Scoring Formula

For agents managing a backlog of detected issues, priority determines processing order:

```
priority = severity * frequency(cap {FREQUENCY_CAP}) * recency_mult * north_star_mult
```

| Factor | Description | Calculation |
|--------|-------------|-------------|
| `severity` | Base severity score (1-5) | From taxonomy default + modifiers |
| `frequency` | Count of occurrences, capped | `min(count, {FREQUENCY_CAP})` |
| `recency_mult` | How recently observed | `{RECENCY_RECENT}` / `{RECENCY_MEDIUM}` / `{RECENCY_OLD}` |
| `north_star_mult` | Alignment with north star metrics | Per-category multiplier (0.5-2.0) |

**Priority classification:**

| Level | Score Range | Action |
|-------|-------------|--------|
| P1 | >= `{P1_THRESHOLD}` | Address immediately, next loop |
| P2 | `{P2_FLOOR}` - `{P1_THRESHOLD}` | Schedule within current cycle |
| P3 | < `{P2_FLOOR}` | Backlog, address in IMPROVE loop |

### Deduplication

Incidents are deduplicated using a composite key:

```
dedup_key = {CATEGORY}:{SCENARIO_ID}:{CONTEXT_RANGE}
```

| Field | Description | Example |
|-------|-------------|---------|
| `{CATEGORY}` | Taxonomy category code | "PERF-DROP" |
| `{SCENARIO_ID}` | Unique identifier for the scenario | Test case ID, transaction ID |
| `{CONTEXT_RANGE}` | Temporal or positional scope | Turn range, time window |

Rules:
- Same dedup_key within `{DEDUP_WINDOW}` = same incident (update, don't create new)
- Different dedup_key = new incident even if same category
- On dedup match: increment frequency, update recency, keep highest severity

### Correlation Detection

When multiple incidents share temporal or causal proximity:

```
IF {CORRELATION_COUNT}+ incidents from different categories occur within {CORRELATION_WINDOW}
THEN flag as potential correlation group
  -> Investigate shared root cause in next INVESTIGATE loop
  -> Apply COMPOUND severity modifier to all members
```

### Evolution Protocol

New incident types emerge from patterns in data. Controlled process:

```
1. Agent observes 3+ similar incidents that don't fit existing types
2. Agent proposes new type on next META loop
3. Proposal includes: code, description, category, evidence (the 3+ incidents)
4. Owner approves or rejects
5. If approved: add to taxonomy, reclassify past incidents
```

### Reference Implementation

This section shows how the abstract taxonomy was applied in a concrete agent managing `{ITEM_COUNT}` categories. This is not prescriptive -- it demonstrates the pattern at scale.

**Category structure (F1-F17 example):**

| ID | Category | Description | North Star Mult |
|----|----------|-------------|-----------------|
| F1 | `{DOMAIN}-ACCURACY-{DETAIL}` | Core accuracy failures | `{HIGH_MULT}` |
| F2 | `{DOMAIN}-COMPLETENESS-{DETAIL}` | Missing required elements | `{HIGH_MULT}` |
| F3 | `{DOMAIN}-CONSISTENCY-{DETAIL}` | Internal contradictions | `{MED_MULT}` |
| F4 | `{DOMAIN}-FORMAT-{DETAIL}` | Output format violations | `{LOW_MULT}` |
| F5 | `{DOMAIN}-BOUNDARY-{DETAIL}` | Edge case handling | `{MED_MULT}` |
| F6 | `{DOMAIN}-REGRESSION-{DETAIL}` | Previously fixed, now broken | `{HIGH_MULT}` |
| F7 | `{DOMAIN}-PERFORMANCE-{DETAIL}` | Speed/resource issues | `{MED_MULT}` |
| F8 | `{DOMAIN}-INTEGRATION-{DETAIL}` | Cross-system failures | `{MED_MULT}` |
| F9 | `{DOMAIN}-VALIDATION-{DETAIL}` | Input validation gaps | `{LOW_MULT}` |
| F10 | `{DOMAIN}-STATE-{DETAIL}` | State management errors | `{MED_MULT}` |
| F11 | `{DOMAIN}-CONCURRENCY-{DETAIL}` | Race conditions, deadlocks | `{HIGH_MULT}` |
| F12 | `{DOMAIN}-CONFIG-{DETAIL}` | Configuration errors | `{LOW_MULT}` |
| F13 | `{DOMAIN}-SECURITY-{DETAIL}` | Security-related failures | `{HIGH_MULT}` |
| F14 | `{DOMAIN}-UX-{DETAIL}` | User experience issues | `{MED_MULT}` |
| F15 | `{DOMAIN}-DATA-{DETAIL}` | Data integrity issues | `{HIGH_MULT}` |
| F16 | `{DOMAIN}-DEPENDENCY-{DETAIL}` | External dependency failures | `{LOW_MULT}` |
| F17 | `{DOMAIN}-UNKNOWN-{DETAIL}` | Uncategorized (trigger evolution) | `{LOW_MULT}` |

**Key observations from this implementation:**
- 17 categories emerged organically over 226 loops (not designed upfront)
- Categories F1-F3 (accuracy, completeness, consistency) consistently had highest north_star_mult
- F6 (regression) requires special handling via anti-flap state machine (see Validation Gates)
- F17 (unknown) acts as a pressure valve -- when it grows beyond `{UNKNOWN_THRESHOLD}` items, it triggers taxonomy evolution
- North star multipliers ranged from `{LOW_MULT}` to `{HIGH_MULT}`, with most categories at `{MED_MULT}`

### Configuration

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `{UNIT_PREFIX}` | Short code per domain | "AFF", "KC", "LOG" | yes |
| `{SCOPE_ALL}` | Definition of "all regions" | "3+ regions" or "all" | yes |
| `{DURATION_THRESHOLD}` | Hours before duration modifier | 4h | yes |
| `{RECURRENCE_WINDOW}` | Days for recurrence check | 7d | yes |
| `{MIN_OBSERVATIONS}` | Incidents needed to propose new type | 3 | yes |
| `{FREQUENCY_CAP}` | Max frequency count in scoring | 5 | yes |
| `{RECENCY_RECENT}` | Multiplier for recent observations | 1.5 | yes |
| `{RECENCY_MEDIUM}` | Multiplier for medium-age observations | 1.0 | yes |
| `{RECENCY_OLD}` | Multiplier for old observations | 0.7 | yes |
| `{P1_THRESHOLD}` | Score >= this = P1 | 8.0 | yes |
| `{P2_FLOOR}` | Score >= this = P2 | 4.0 | yes |
| `{DEDUP_WINDOW}` | Time window for deduplication | 1 cycle | yes |
| `{CORRELATION_COUNT}` | Min incidents for correlation group | 3 | yes |
| `{CORRELATION_WINDOW}` | Time proximity for correlation | 2 loops | yes |
| `{HIGH_MULT}` | North star multiplier for critical categories | 2.0 | yes |
| `{MED_MULT}` | North star multiplier for standard categories | 1.0 | yes |
| `{LOW_MULT}` | North star multiplier for minor categories | 0.5 | yes |
| `{UNKNOWN_THRESHOLD}` | Max unknown items before evolution trigger | 5 | no |

### Decision Rules

- **New category vs existing**: If an incident fits an existing category with > 70% confidence, use existing. Otherwise, file under `{DOMAIN}-UNKNOWN-{DETAIL}` and let evolution protocol handle it.
- **North star multiplier assignment**: Categories directly affecting `{NORTH_STAR_METRICS}` get `{HIGH_MULT}`. Indirectly related get `{MED_MULT}`. Infrastructure/tooling get `{LOW_MULT}`.
- **Dedup vs new**: Same dedup_key within window = update existing. Different key = new incident even if same category.
- **Correlation trigger**: `{CORRELATION_COUNT}`+ incidents from `{CORRELATION_COUNT}`+ different categories within `{CORRELATION_WINDOW}` = potential root cause.

<!-- REFERENCE: load on adoption -->
## Adaptation Guide

### Prerequisites
- Clear domain decomposition into units
- Incident entity model with type, severity, confidence fields
- A monitoring loop that produces incidents
- North star metrics defined (for priority scoring)

### Steps to Adopt
1. Define unit prefixes (2-4 chars each)
2. Start with 5-10 core types per unit (don't over-engineer upfront)
3. Always include `CROSS-*` and `DATA-*` categories from day one
4. Define severity modifier conditions
5. Set evolution protocol: how many observations -> propose -> approve
6. Implement priority scoring formula with initial north_star_mult values (default all to `{MED_MULT}`)
7. Add deduplication with composite keys
8. Review and evolve the taxonomy every `{MIN_OBSERVATIONS}` META loops
9. Tune north_star_mult values based on observed impact after 2-3 cycles
10. Add correlation detection once taxonomy has 10+ categories

### What to Customize
- Unit prefixes and per-unit incident types (fully domain-specific)
- Specific modifier conditions and thresholds
- Number of observations required for evolution
- North star multiplier values per category
- Priority thresholds (P1/P2/P3 score boundaries)
- Dedup key structure (which fields compose the key)
- Correlation detection parameters

### What NOT to Change
- `{UNIT}-{CATEGORY}-{DETAIL}` naming convention
- `CROSS-*` and `DATA-*` as mandatory cross-cutting categories
- Orthogonal severity modifiers (don't bake severity into type definition)
- Evolution protocol requiring observations + approval (don't add types speculatively)
- Priority = severity * frequency * recency * north_star (don't simplify to just severity)
- Deduplication via composite key (don't rely on title matching)
- Capped frequency in scoring formula (uncapped frequency dominates all other factors)

<!-- HISTORY: load for audit -->
## Origin

### hilart-ops-bot
- **Findings:** [2] Incident Taxonomy Structure
- **Discovered through:** 40+ incident types developed over 9 loops. Starting with loose descriptions, then formalizing into hierarchical codes. Evolution protocol added to prevent taxonomy bloat -- types only added after 3+ real observations.
- **Evidence:** 6 categories (AFF, KC, LOG, CHAT, CROSS, DATA) covered all observed anomalies. Severity modifiers enabled nuanced escalation (same type, different severity based on context).

### voic-experiment
- **Findings:** [60] F1-F17 Failure Taxonomy, [49] Priority Scoring Formula
- **Discovered through:** 226 loops producing a 17-category failure taxonomy. Priority scoring formula developed to handle 50+ active backlog items -- raw severity alone was insufficient for ordering work. North star multipliers added to align failure prioritization with overall agent objectives. Dedup keys prevented the same root cause from appearing as multiple backlog items.
- **Evidence:** Priority scoring reduced average time-to-fix for P1 items by focusing agent attention. F17 (unknown) category successfully triggered 4 taxonomy evolution events, each adding 1-2 new categories. Correlation detection identified 3 root-cause clusters that individual category tracking missed.

## Related Patterns
- [Confidence & Severity Model](../methodologies/confidence_severity.md) -- severity calculation uses taxonomy types
- [Autonomous Loop Protocol](../patterns/loop_protocol.md) -- DETECT phase classifies per taxonomy
- [Data Quality Framework](../methodologies/data_quality.md) -- DATA-* types come from data quality checks
- [Closed-Loop Quality System](../methodologies/closed_loop_quality.md) -- generates failures that feed into taxonomy
- [Validation Gates](../patterns/validation_gates.md) -- anti-flap state machine handles regression (F6) category
