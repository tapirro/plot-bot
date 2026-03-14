---
id: cross-confidence-severity
title: Confidence & Severity Model
type: methodology
concern: [confidence-scoring]
mechanism: [scoring-model, state-machine]
scope: per-item
lifecycle: [detect, classify, decide]
origin: harvest/hilart-ops-bot
origin_findings: [5, 6, 8, 17]
maturity: draft
domain: cross-agent/methodologies
confidence: observed
basis: "distilled from hilart-ops-bot harvest, 42 findings from ops bot sessions"
---

# Confidence & Severity Model

<!-- CORE: load always -->
## Problem

Without quantified confidence, agents face a binary choice on every detected anomaly: escalate or ignore. This produces two failure modes, both costly. Over-escalation floods human operators with noise -- every anomaly is treated as urgent regardless of how certain the agent is that it represents a real issue. Under-escalation silently drops real problems because the agent has no framework for distinguishing a low-confidence detection from a low-severity one.

The problem compounds when investigations are disconnected from the incidents that triggered them. An agent investigates an anomaly, determines it was a false alarm, but the original incident remains in the system with its initial severity and confidence unchanged. Without a write-back mechanism, the same false alarm gets re-escalated on the next cycle, and investigations produce knowledge that never flows back to improve future detection.

Confidence is not a static property -- it changes as data ages, as fresh observations confirm or contradict the initial detection, and as investigations conclude. Systems that treat confidence as a one-time assignment at detection miss these dynamics entirely.

## Solution

Every incident carries two orthogonal scores: severity (how bad the issue is, if real) and confidence (how certain the system is that the issue is real). These scores serve different gating functions: severity determines what action to take (ignore, log, escalate), while confidence determines whether to take that action at all. A high-severity, low-confidence incident triggers investigation rather than escalation.

Confidence is a first-class citizen with a full lifecycle. Initial confidence is assigned from a matrix of metric class (INSTANT, SAME_DAY, LAGGING) crossed with data freshness (fresh, moderate, stale). It then evolves: unchecked incidents lose confidence through time-based decay; fresh observations that confirm the anomaly increase confidence; data pipeline outages halve confidence; and cross-correlation with secondary sources adds incremental confidence.

When severity is high but confidence falls below a configurable threshold, investigation is triggered automatically. The investigation follows a budgeted protocol with checkpointed sessions. Crucially, investigation conclusions write back to the parent incident: a false alarm reduces the parent's confidence, a confirmed issue raises it, and an inconclusive investigation preserves a floor confidence level. This write-back contract closes the loop and prevents orphan incidents from cycling through repeated escalation.

Escalation is gated on confidence: no incident is escalated unless confidence meets or exceeds the escalation threshold, regardless of severity. This single gate eliminates premature escalation while ensuring that confirmed high-severity issues reach human operators promptly. Rate limiting and deduplication prevent escalation storms.

## Implementation

### Structure

```
INCIDENT
├── severity: INFO | LOW | MEDIUM | HIGH | CRITICAL
├── confidence: float (0.0 - 1.0)
├── confidence_source: "detection" | "investigation" | "recheck" | "decay"
├── needs_investigation: boolean
└── investigation_id: string | null (link to INV-NNN)
```

### Severity Calculation

Base severity from threshold deviation:

```
abs_deviation >= {CRITICAL_THRESHOLD} × 1.5  → CRITICAL
abs_deviation >= {CRITICAL_THRESHOLD}         → HIGH
abs_deviation >= {WARNING_THRESHOLD} × 1.5    → MEDIUM
abs_deviation >= {WARNING_THRESHOLD}           → LOW
else                                           → INFO
```

Orthogonal modifiers (each ±1 level):

| Modifier | Condition | Effect |
|----------|-----------|--------|
| Scope | Affects all regions/units | +1 |
| Duration | Persists > `{DURATION_HOURS}` | +1 |
| Compound | 2+ simultaneous incidents | +1 |
| Recurring | 3+ occurrences in `{RECURRENCE_WINDOW}` | +1 |

### Initial Confidence Assignment

Matrix of `metric_class × data_freshness`:

| Metric Class | Fresh (< `{FRESH_HOURS}`) | Moderate | Stale (> `{STALE_HOURS}`) |
|-------------|---------------------------|----------|---------------------------|
| INSTANT | 0.85 | 0.70 | 0.50 |
| SAME_DAY | 0.75 | 0.60 | 0.40 |
| LAGGING | 0.30 | 0.30 | 0.30 |

### Confidence Modifiers

| Condition | Effect |
|-----------|--------|
| Data pipeline DOWN | ×0.5 |
| Cross-correlation confirms | +0.10 |
| Secondary source confirms | +0.10 |
| Investigation concluded | Use investigation's confidence |

### Confidence Decay

Unchecked incidents lose confidence over time:

| Metric Class | Decay Rate | Floor |
|-------------|-----------|-------|
| INSTANT | -0.10 per `{DECAY_INTERVAL}` | 0.20 |
| SAME_DAY | -0.05 per `{DECAY_INTERVAL}` | 0.20 |
| LAGGING | -0.02 per `{DECAY_INTERVAL}` | 0.20 |

### Confidence Lifecycle (Closed Loop)

```
DETECT → assign initial confidence
  → DECAY → erode if unchecked (per decay rate)
  → RECHECK → update from fresh data
       persistent: +0.05, worsening: +0.10
  → TRIGGER → auto needs_investigation
       IF severity >= {INVESTIGATION_SEVERITY} AND confidence < {INVESTIGATION_THRESHOLD}
  → TRIAGE → promote to investigation backlog
  → INVESTIGATE → hypothesize → query → analyze → checkpoint
  → WRITE-BACK → update parent incident from investigation
  → ESCALATE → confidence gate check
  → CLOSE → human review
```

### Write-Back Contract

Investigation conclusion MUST update parent incident:

| Investigation Result | Parent Update |
|---------------------|---------------|
| FALSE ALARM | parent.confidence = inv.confidence, needs_investigation = false |
| Confirmed | parent.confidence = inv.confidence, needs_investigation = false |
| No root cause found | parent.confidence = max(0.5, current), needs_investigation remains |

### Investigation Budget

| Priority | Max Sessions | Max Time per Session |
|----------|-------------|---------------------|
| P1 | `{P1_SESSIONS}` | `{SESSION_MINUTES}` min |
| P2 | `{P2_SESSIONS}` | `{SESSION_MINUTES}` min |
| P3 | `{P3_SESSIONS}` | `{SESSION_MINUTES}` min |

Checkpoint decisions: `CONCLUDED` / `OPEN` / `BLOCKED`

### Escalation Gates

| Severity | Action | Confidence Gate |
|----------|--------|----------------|
| INFO, LOW | Loop report / daily digest | none |
| MEDIUM | Daily digest, flagged | none |
| HIGH or 3+ MEDIUM | Immediate escalation | confidence >= `{ESCALATION_THRESHOLD}` |
| CRITICAL | Immediate + escalation flag | confidence >= `{ESCALATION_THRESHOLD}` |

Rate limit: max `{MAX_ESCALATIONS_HOUR}` per hour, no duplicate within `{DEDUP_HOURS}` hours.

**If confidence < `{ESCALATION_THRESHOLD}` → BLOCKED. Must investigate first.**

### Configuration

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `{FRESH_HOURS}` | Hours before data is "moderate" | 6 | yes |
| `{STALE_HOURS}` | Hours before data is "stale" | 24 | yes |
| `{DECAY_INTERVAL}` | Time unit for decay | 6h | yes |
| `{INVESTIGATION_SEVERITY}` | Min severity for auto-investigation | MEDIUM | yes |
| `{INVESTIGATION_THRESHOLD}` | Confidence below which investigation triggers | 0.6 | yes |
| `{ESCALATION_THRESHOLD}` | Confidence required for escalation | 0.6 | yes |
| `{DURATION_HOURS}` | Hours before duration modifier applies | 4 | yes |
| `{RECURRENCE_WINDOW}` | Days for recurrence modifier | 7 | yes |
| `{P1_SESSIONS}` | Investigation session budget for P1 | 5 | yes |
| `{P2_SESSIONS}` | Investigation session budget for P2 | 3 | yes |
| `{P3_SESSIONS}` | Investigation session budget for P3 | 2 | yes |
| `{SESSION_MINUTES}` | Max minutes per investigation session | 15 | yes |
| `{MAX_ESCALATIONS_HOUR}` | Rate limit for escalations | 5 | yes |
| `{DEDUP_HOURS}` | Deduplication window for escalations | 4 | yes |

<!-- REFERENCE: load on adoption -->
## Adaptation Guide

### Prerequisites
- Metric Classification Framework adopted (needed for initial confidence matrix)
- Incident entity model with severity and confidence fields
- Investigation tracking (backlog, session management)
- Escalation channel configured

### Steps to Adopt
1. Add `confidence`, `confidence_source`, `needs_investigation` fields to incident entity
2. Implement initial confidence assignment matrix (metric_class × data_freshness)
3. Implement severity calculation (threshold deviation + orthogonal modifiers)
4. Add confidence decay timer (run every `{DECAY_INTERVAL}`)
5. Add recheck logic (on fresh data, adjust confidence up)
6. Add auto-investigation trigger rule
7. Create investigation backlog with priority and session budget
8. Implement write-back: investigation result → parent incident update
9. Add escalation confidence gate
10. Add rate limiting and deduplication to escalation

### What to Customize
- Initial confidence matrix values (calibrate to your domain)
- Decay rates per metric class
- Investigation/escalation thresholds
- Session budget per priority
- Severity modifier conditions (scope, duration, compound, recurring)
- Rate limits

### What NOT to Change
- Two orthogonal scores (severity + confidence) — don't merge them
- Confidence as lifecycle, not snapshot — must decay, recheck, write-back
- Write-back contract — investigation MUST update parent incident
- Escalation gate — confidence below threshold MUST block escalation
- Investigation trigger rule — low confidence + high severity = investigate

<!-- HISTORY: load for audit -->
## Origin
- **Source agent:** hilart-ops-bot
- **Findings:** [5] Confidence Model, [6] Confidence Lifecycle, [8] Investigation Closed Loop, [17] Severity Calculation
- **Discovered through:** v0.9.3-v0.9.4 evolution. Early versions had severity but no confidence → escalated false positives. Adding quantified confidence with escalation gate prevented premature escalation. Write-back contract (v0.9.4) closed the loop: investigations that disprove incidents reduce parent confidence, preventing re-escalation.
- **Evidence:** S/N improved 0.2:1 → 1.5:1 over 9 loops. Escalation quality improved by gating on confidence >= 0.6. 4 concluded investigations (INV-001 through INV-004) each produced write-back updates. Orphan incidents eliminated by write-back contract.

## Related Patterns
- [Metric Classification](../methodologies/metric_classification.md) — provides metric class for confidence matrix
- [Autonomous Loop Protocol](../patterns/loop_protocol.md) — loop structure where confidence lifecycle operates
- [Monitoring Principles](../best_practices/monitoring_principles.md) — why suppression matters
- [Incident Taxonomy](../patterns/incident_taxonomy.md) — taxonomy types determine severity calculation
- [Validation Gates](../patterns/validation_gates.md) — escalation gate uses confidence threshold
- [Closed-Loop Quality System](../methodologies/closed_loop_quality.md) — quality pipeline generates severity scores
- [Data Quality Framework](../methodologies/data_quality.md) — data quality checks feed into confidence
- [Adversarial Reflection](../patterns/adversarial_reflection.md) -- adversarial review can refine confidence scores through opposing perspectives
