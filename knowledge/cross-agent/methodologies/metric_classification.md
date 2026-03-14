---
id: cross-metric-classification
title: Metric Classification Framework
type: methodology
concern: [metric-comparison]
mechanism: [taxonomy, scoring-model]
scope: per-item
lifecycle: [classify, detect]
origin: harvest/hilart-ops-bot
origin_findings: [4]
maturity: draft
domain: cross-agent/methodologies
confidence: observed
basis: "distilled from hilart-ops-bot harvest, 42 findings from ops bot sessions"
---

# Metric Classification Framework

<!-- CORE: load always -->
## Problem

Not all metrics behave the same way over time. A queue size is meaningful at any instant -- if it exceeds a threshold right now, that is a real signal. A daily order count accumulates through the day -- comparing a 10 AM reading to yesterday's end-of-day total produces a guaranteed false positive. A 21-day delivery rate requires weeks of data to mature -- comparing today's partial data to a stable baseline is meaningless.

When an agent treats all metrics equally -- applying the same comparison method, the same baseline window, and the same alert thresholds -- it generates noise that drowns out real signals. In practice, this manifests as a near-100% false positive rate in early monitoring loops, with the agent escalating data artifacts as business crises. The signal-to-noise ratio drops below 1:1, making the monitoring system worse than useless: it actively wastes human attention on phantom issues.

## Solution

Classify every monitored metric into one of three temporal classes: INSTANT (value meaningful at any moment, compared against absolute thresholds), SAME_DAY (accumulates through the day, compared against hour-normalized baselines), and LAGGING (requires days or weeks to mature, with alerts auto-suppressed until data reaches maturation age).

The classification is performed once during metric setup and rarely changes. Each class defines its own comparison method, baseline calculation window, and alert behavior. INSTANT metrics alert immediately on threshold breach. SAME_DAY metrics compute an expected value as `baseline_average * (current_hour / 24)` before comparing. LAGGING metrics suppress alerts to INFO level when data age is below the configured maturation period, preventing premature escalation of immature data.

This single change -- classifying metrics before comparing them -- eliminates the dominant source of false positives. In the originating system, it reduced the false positive rate from 90% to approximately 12% in one iteration, improving signal-to-noise from 0.2:1 to 0.88:1.

## Implementation

### Structure

```
METRIC
├── name: string
├── class: INSTANT | SAME_DAY | LAGGING
├── baseline_window: int (days)
├── threshold_warning: float
├── threshold_critical: float
├── direction: "up_is_bad" | "down_is_bad"
└── maturation_days: int (LAGGING only)
```

Three classes:

| Class | Data Nature | Comparison Method | Alert Behavior |
|-------|------------|-------------------|----------------|
| **INSTANT** | Value meaningful at any moment | Absolute thresholds | Alert immediately when threshold exceeded |
| **SAME_DAY** | Accumulates through the day | Hour-normalized baseline | `expected = baseline_{N}d × (current_hour / 24)` |
| **LAGGING** | Requires days/weeks to mature | Suppressed intraday | Auto-suppress to INFO if data age < `{MATURATION_DAYS}` |

### Configuration

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `{METRIC_NAME}` | Name of the metric | "queue_size", "delivery_rate" | yes |
| `{CLASS}` | INSTANT, SAME_DAY, or LAGGING | SAME_DAY | yes |
| `{BASELINE_WINDOW}` | Days of history for baseline | 7 | yes |
| `{THRESHOLD_WARNING}` | Warning deviation threshold | 0.2 (20%) | yes |
| `{THRESHOLD_CRITICAL}` | Critical deviation threshold | 0.5 (50%) | yes |
| `{MATURATION_DAYS}` | Days until data is mature (LAGGING only) | 21 | for LAGGING |

### Decision Rules

```
FOR each metric observation:
  1. Look up metric.class
  2. IF class == INSTANT:
       deviation = |current - threshold| / threshold
       → alert if exceeds warning/critical
  3. IF class == SAME_DAY:
       expected = baseline_{N}d_avg × (current_hour / 24)
       deviation = |current - expected| / expected
       → alert if exceeds warning/critical
  4. IF class == LAGGING:
       IF data_age < maturation_days:
         → suppress to INFO (do not alert)
       ELSE:
         → compare to mature baseline only
```

<!-- REFERENCE: load on adoption -->
## Adaptation Guide

### Prerequisites
- A list of metrics the agent monitors
- Historical data for baseline calculation (minimum: `{BASELINE_WINDOW}` days)
- Understanding of each metric's temporal behavior

### Steps to Adopt
1. List all monitored metrics
2. For each metric, determine: does it make sense to compare at any moment (INSTANT), does it accumulate through the day (SAME_DAY), or does it require days to stabilize (LAGGING)?
3. Set `{BASELINE_WINDOW}` (typically 7d for INSTANT/SAME_DAY, 30d for LAGGING)
4. For LAGGING metrics, determine `{MATURATION_DAYS}` — how long until data is meaningful
5. Add classification to metric definitions
6. Update comparison logic to branch by class

### What to Customize
- Which metrics are INSTANT vs SAME_DAY vs LAGGING (domain-specific)
- Baseline window per metric
- Maturation days for LAGGING metrics
- Specific thresholds per metric

### What NOT to Change
- The three-class taxonomy itself (INSTANT / SAME_DAY / LAGGING)
- Hour-normalization formula for SAME_DAY: `baseline × (hour / 24)`
- Auto-suppression of immature LAGGING data to INFO
- The principle: classification happens once, comparison logic branches per class

<!-- HISTORY: load for audit -->
## Origin
- **Source agent:** hilart-ops-bot
- **Findings:** [4] Metric Classification Framework
- **Discovered through:** 9 monitoring loops with 100% false positive rate in loops 1-3. Root cause: comparing intraday partial-day data to 7d full-day baselines for delivery rate (a LAGGING metric). The fix in v0.9.2 was a single change — classify metrics — which dropped FP from 90% to ~12%.
- **Evidence:** S/N improved 0.2:1 → 0.88:1 in one iteration. 9/10 false positives eliminated.

## Related Patterns
- [Confidence & Severity Model](../methodologies/confidence_severity.md) — uses metric class for initial confidence assignment
- [Data Quality Framework](../methodologies/data_quality.md) — pre-check before metric comparison
- [Autonomous Loop Protocol](../patterns/loop_protocol.md) — determines comparison method in DETECT phase
- [Monitoring Principles](../best_practices/monitoring_principles.md) — metric classification is the #1 suppression mechanism
