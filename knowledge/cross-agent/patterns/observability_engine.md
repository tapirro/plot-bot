---
id: cross-observability-engine
title: Observability Engine
type: pattern
concern: [observability]
mechanism: [pipeline, registry]
scope: per-item
lifecycle: [detect, reflect]
origin: harvest/voic-experiment
origin_findings: [1, 3, 13, 57, 58]
maturity: draft
domain: cross-agent/patterns
confidence: observed
basis: "distilled from voic-experiment harvest, 65 findings from voice agent sessions"
---

# Observability Engine

<!-- CORE: load always -->
## Problem

Multi-stage pipelines lack visibility into where time is spent, where errors occur, and what changed between runs. Aggregate metrics (total duration, overall error rate) hide stage-level distribution, making optimization directionless -- an agent cannot improve what it cannot measure at the right granularity.

Adding measurement points ad hoc breaks existing dashboards and analysis scripts because there is no contract for backward compatibility. Cold starts (initialization, cache misses) skew P50 calculations, causing agents to waste effort optimizing one-time overhead that does not affect steady-state performance.

The most insidious failure mode is the unmeasured "Other" category. When a pipeline measures individual stages but not the gaps between them, the unmeasured portion can silently dominate total duration. An agent optimizing the most expensive visible stage may be addressing 15% of total time while 60% hides in unmeasured transitions. Without explicit "Other" decomposition, this remains invisible indefinitely.

## Solution

A span-based tracing system where every pipeline stage is wrapped in a span that captures start time, end time, status, and arbitrary metadata. Spans nest automatically via a stack-based context manager API, ensuring they are always properly closed even on error. Parent-child relationships enable "Other" decomposition: parent duration minus the sum of child durations reveals unmeasured work.

A measurement point map serves as a registry of all instrumented stages with stable IDs. New measurement points are always added, never removed or renamed, guaranteeing backward compatibility for historical data and analysis scripts.

Cold and warm span categories are tracked separately. The first invocation of each component in a session is marked "cold"; subsequent invocations are "warm." Percentile analysis defaults to warm spans only, giving an accurate picture of steady-state performance. Cold span analysis is available separately for initialization optimization.

Dual-track JSONL logging separates concerns: a conversation log captures high-level events (pipeline start/end, errors, decisions) for human debugging, while a trace log captures full span data for machine analysis. A circular buffer provides bounded in-memory storage for recent traces, preventing memory exhaustion in long-running sessions while JSONL persistence captures the complete record.

Built-in analysis functions compute percentiles (P50, P90, P99), compare baselines against current performance with delta percentages, and decompose "Other" to identify when unmeasured portions exceed a configurable threshold. These functions provide the quantitative foundation for hypothesis-driven optimization.

## Implementation

### Structure

```
OBSERVABILITY_ENGINE
├── span_registry: MAP<string, SpanDefinition>
├── active_spans: STACK<Span>
├── buffer: CircularBuffer<TraceEntry>({BUFFER_SIZE})
├── persistence:
│   ├── conversation_log: JSONL file    ← high-level events per session
│   └── trace_log: JSONL file           ← detailed span data per session
└── analysis:
    ├── percentiles(metric, [P50, P90, P99])
    └── compare(metric, baseline, current)
```

### Span Model

```
SPAN
├── id: string (auto-generated)
├── name: string                       ← from registry
├── parent_id: string | null           ← for nested spans
├── category: "cold" | "warm"          ← initialization vs steady-state
├── start_time: float (monotonic)
├── end_time: float | null
├── duration_ms: float | null          ← computed on close
├── metadata: object                   ← arbitrary key-value pairs
├── status: "ok" | "error"
└── error_message: string | null
```

### Context Manager API

Spans are created and closed via a context manager, ensuring they are always properly closed even on error:

```
WITH trace("{STAGE_NAME}", category="warm") AS span:
    span.set_metadata("{KEY}", {VALUE})
    ... do work ...
    # span auto-closes with duration and status
```

Nesting is automatic — a span opened inside another span becomes its child:

```
WITH trace("{PARENT_STAGE}") AS parent:
    WITH trace("{CHILD_STAGE_A}") AS child_a:
        ... stage A work ...
    WITH trace("{CHILD_STAGE_B}") AS child_b:
        ... stage B work ...
    # parent.duration includes both children
    # "Other" = parent.duration - child_a.duration - child_b.duration
```

### Measurement Point Map

A registry of all measurement points in the pipeline. Each point has a stable ID, a human-readable name, and its position in the pipeline.

```
MEASUREMENT_POINT_MAP
├── total_points: {TOTAL_POINTS}
├── total_segments: {TOTAL_SEGMENTS}   ← points - 1 (gaps between points)
├── points:
│   ├── MP-001: {name: "{STAGE_1}", position: 0, added_in_cycle: 1}
│   ├── MP-002: {name: "{STAGE_2}", position: 1, added_in_cycle: 1}
│   ├── ...
│   └── MP-{NNN}: {name: "{STAGE_N}", position: N-1, added_in_cycle: K}
```

**Backward compatibility rule:** New measurement points are always ADDED, never removing or renaming existing ones. This ensures:
- Historical data remains valid
- Dashboards and analysis scripts do not break
- "Other" calculation adjusts automatically (new sub-spans reduce "Other")

### Cold vs. Warm Tracking

| Category | What It Captures | When to Use |
|----------|-----------------|-------------|
| Cold | Initialization, first-run overhead, cache misses | First invocation of a component in a session |
| Warm | Steady-state processing, cache hits | Subsequent invocations after initialization |

**Why this matters:** If cold spans are mixed with warm spans in percentile calculations, P50 appears worse than steady-state reality. An agent optimizing P50 might waste effort on initialization overhead that only occurs once per session.

**Rules:**
- Mark the first invocation of each pipeline component as `cold`
- Mark all subsequent invocations as `warm`
- Percentile analysis defaults to `warm` spans only
- Cold span analysis is available separately for initialization optimization
- Reports show both: `P50 (warm): {X}ms, P50 (cold): {Y}ms`

### Dual-Track Logging

Two JSONL files per session, serving different purposes:

**Conversation log** (`{LOG_DIR}/{SESSION_ID}_conversation.jsonl`):
- High-level events: session start/end, pipeline run start/end, errors, decisions
- One line per event
- Human-readable when tailed
- Used for: quick debugging, session overview, anomaly detection

```json
{"ts": "{TIMESTAMP}", "event": "pipeline_start", "run_id": "{RUN_ID}", "input_count": {N}}
{"ts": "{TIMESTAMP}", "event": "stage_complete", "stage": "{STAGE_NAME}", "duration_ms": {D}, "status": "ok"}
{"ts": "{TIMESTAMP}", "event": "pipeline_end", "run_id": "{RUN_ID}", "total_ms": {T}, "status": "ok"}
```

**Trace log** (`{LOG_DIR}/{SESSION_ID}_trace.jsonl`):
- Full span data: all fields from the Span model
- One line per completed span
- Machine-readable, used for percentile analysis and comparison
- Used for: performance analysis, regression detection, "Other" decomposition

```json
{"span_id": "{ID}", "name": "{STAGE}", "parent_id": "{PID}", "category": "warm", "duration_ms": {D}, "status": "ok", "metadata": {}}
```

### Circular Buffer

In-memory storage with fixed capacity, preventing unbounded memory growth:

```
CIRCULAR_BUFFER
├── capacity: {BUFFER_SIZE}
├── entries: array of TraceEntry
├── head: int                          ← next write position
└── count: int                         ← current fill level
```

**Behavior:**
- New entries overwrite oldest when buffer is full
- Provides iteration over most recent `{BUFFER_SIZE}` entries
- JSONL persistence captures all entries (buffer is for in-memory analysis only)
- Typical sizing: `{BUFFER_SIZE}` = 200 entries covers multiple pipeline runs

### Analysis Functions

**Percentiles:**

```
percentiles(metric_name, quantiles=[P50, P90, P99], category="warm")
  → {P50: value, P90: value, P99: value, count: N}
```

Filters by `category` (default: warm). Requires minimum `{MIN_SAMPLES}` entries; returns null if insufficient data.

**Compare:**

```
compare(metric_name, baseline_data, current_data)
  → {
      metric: name,
      baseline: {P50, P90, P99, count},
      current: {P50, P90, P99, count},
      delta: {P50: diff, P90: diff, P99: diff},
      delta_pct: {P50: pct, P90: pct, P99: pct},
      improved: boolean (true if P50 decreased)
    }
```

Used at cycle end for hypothesis verification (see Hypothesis-Driven Experimentation).

**"Other" Decomposition:**

```
decompose_other(parent_span_name)
  → {
      parent_duration: value,
      measured_children: [{name, duration, pct_of_parent}],
      other_duration: value,
      other_pct: float,
      recommendation: "instrument" | "acceptable"
    }

recommendation = "instrument" IF other_pct > {OTHER_THRESHOLD}%
```

### Decision Rules

**When to add measurement points:**
- Before optimizing any pipeline stage — verify it is the actual top offender
- When "Other" exceeds `{OTHER_THRESHOLD}%` of parent span — decompose the unmeasured portion
- When a new pipeline stage is added — register it in the measurement point map immediately

**When NOT to add measurement points:**
- Mid-optimization — changing measurement while changing behavior confounds analysis
- For temporary debugging — use metadata on existing spans instead
- Inside tight loops — span overhead becomes significant at sub-millisecond granularity

**Span overhead budget:**
- Each span adds approximately `{SPAN_OVERHEAD}` of overhead
- If pipeline has `{TOTAL_POINTS}` measurement points, total overhead is approximately `{TOTAL_POINTS} × {SPAN_OVERHEAD}`
- If overhead exceeds `{MAX_OVERHEAD_PCT}%` of total pipeline time, consolidate low-value spans

### Configuration

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `{BUFFER_SIZE}` | Circular buffer capacity (entries) | 200 | yes |
| `{MIN_SAMPLES}` | Minimum samples for percentile calculation | 5 | yes |
| `{OTHER_THRESHOLD}` | "Other" % triggering instrumentation recommendation | 30 | yes |
| `{LOG_DIR}` | Directory for JSONL log files | "logs/" | yes |
| `{SESSION_ID}` | Unique identifier per session | "2026-03-04T10:00" | yes |
| `{TOTAL_POINTS}` | Number of measurement points in registry | 22 | informational |
| `{TOTAL_SEGMENTS}` | Number of measured segments (points - 1) | 21 | informational |
| `{SPAN_OVERHEAD}` | Approximate overhead per span | "0.1ms" | informational |
| `{MAX_OVERHEAD_PCT}` | Maximum acceptable measurement overhead | 1 | yes |

<!-- REFERENCE: load on adoption -->
## Adaptation Guide

### Prerequisites
- A multi-stage pipeline or workflow to instrument
- A context manager or equivalent scoping mechanism in your language
- File system access for JSONL persistence
- A concept of "sessions" or "runs" to scope trace data

### Steps to Adopt
1. Define your measurement point map: list all pipeline stages and assign stable IDs
2. Implement the Span model with id, name, parent_id, category, timing, status
3. Implement the context manager API (trace function that auto-closes spans)
4. Implement automatic parent-child nesting (stack-based active span tracking)
5. Add cold/warm categorization: mark first invocation as cold, subsequent as warm
6. Implement the circular buffer with configurable capacity
7. Implement dual-track JSONL logging (conversation + trace)
8. Implement percentiles function (filter by category, require minimum samples)
9. Implement compare function (baseline vs. current with delta and delta_pct)
10. Implement decompose_other function (parent - sum of children = Other)
11. Register all existing pipeline stages in the measurement point map
12. Run the pipeline and verify spans are captured correctly
13. Establish baselines: run `{MIN_SAMPLES}` iterations and record initial percentiles

### What to Customize
- Measurement point names and positions (match your pipeline stages)
- Buffer size (increase for longer sessions, decrease for memory-constrained environments)
- Log directory and naming convention
- Session ID format
- Metadata fields on spans (add domain-specific context)
- "Other" threshold (lower for precision-critical pipelines, higher for rapid development)
- Minimum samples for percentile calculation
- Span overhead budget (adjust based on pipeline granularity)

### What NOT to Change
- Span as the atomic unit — do not use counters or aggregates as the base; they lose distribution information
- Context manager API — manual span open/close leads to leaked spans on errors
- Cold vs. warm separation — mixing them produces misleading P50 values
- Backward-compatible point addition — never remove or rename measurement points
- Dual-track logging — conversation log for humans, trace log for machines; merging them serves neither well
- Circular buffer for in-memory + JSONL for persistence — unbounded in-memory storage eventually causes issues
- "Other" decomposition — this is the mechanism that prevents optimizing the wrong thing
- Percentiles over means — means hide outliers and multimodal distributions

<!-- HISTORY: load for audit -->
## Origin
- **Source agent:** voic-experiment
- **Findings:** [1] Span-Based Tracing Architecture, [3] Measurement Point Map (22 points, 21 segments), [13] Cold vs. Warm Span Tracking, [57] Dual-Track Logging, [58] Circular Buffer with JSONL Persistence
- **Discovered through:** progressive instrumentation of a multi-stage processing pipeline. Started with a single total-time measurement. Added stage-level spans when optimization was directionless. The measurement point map grew from 5 to 22 points as "Other" decomposition repeatedly revealed hidden bottlenecks. Cold/warm separation was added after noticing P50 was inflated by initialization overhead. Dual-track logging emerged from the tension between human-readable logs (for debugging) and machine-readable traces (for analysis). The circular buffer was added after an extended session exhausted memory with unbounded trace storage.
- **Evidence:** 22 measurement points across the pipeline generating 21 measured segments. "Other" decomposition revealed a 72% unmeasured bottleneck that was invisible to stage-level analysis. Cold/warm separation showed a 3x difference between first-run and steady-state P50. Backward-compatible point addition maintained all historical comparisons across 40+ cycles of measurement evolution.

## Related Patterns
- [Hypothesis-Driven Experimentation](../methodologies/experimentation.md) — uses compare() for before/after verification and decompose_other() for the "Other" rule
- [Three-Tier Backlog Management](../methodologies/backlog_management.md) — observability gaps generate auto-backlog items
- [Human Review Digest](../patterns/human_review_digest.md) — observability data supports `expected` claims in digest items
- [Autonomous Loop Protocol](../patterns/loop_protocol.md) — spans instrument loop phases; self-metrics derive from trace data
- [Monitoring Principles](../best_practices/monitoring_principles.md) — instrumentation infrastructure for "Instrument Before Optimizing" principle
- [Artifact-Centric Interface](../patterns/artifact_centric_interface.md) — trace data as JSONL follows Layer 1
