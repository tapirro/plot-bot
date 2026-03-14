---
id: cross-closed-loop-quality
title: Closed-Loop Quality System
type: methodology
concern: [confidence-scoring, validation-gating]
mechanism: [pipeline, state-machine]
scope: per-cycle
lifecycle: [detect, classify, decide, act]
origin: harvest/voic-experiment
origin_findings: [16, 28, 29, 30, 31]
maturity: draft
domain: cross-agent/methodologies
confidence: observed
basis: "distilled from voic-experiment harvest, 65 findings from voice agent sessions"
---

# Closed-Loop Quality System

<!-- CORE: load always -->
## Problem

Test suites catch regressions, but raw results -- "47 passed, 3 failed" -- provide no machinery for acting on failures systematically. The results sit in CI logs with no classification, no prioritization, and no tracking of whether a failure is new, recurring, or already being worked on.

Without a closed loop, failures accumulate silently. The same issue reappears across multiple runs but is treated as a new problem each time. Fixes that appear to resolve a failure are not verified across subsequent runs, so regressions oscillate: an item is "fixed," reappears two runs later, gets "fixed" again, and the cycle repeats. Teams cannot distinguish between a test that failed once due to flakiness and a test that has failed consistently for weeks.

Effort is misallocated because there is no severity scoring or priority ranking. A minor formatting issue gets the same attention as a critical correctness failure. The agent cannot answer basic questions: "Are things getting better or worse? What should I fix first? Did last cycle's fix actually hold?"

## Solution

A three-stage pipeline -- test, classify, manage -- converts raw test results into a managed backlog with automated prioritization, deduplication, and regression tracking. The test runner produces structured results with stable test IDs. The failure analyzer classifies each failure by category and severity using rule-based pattern matching, LLM-evaluated scoring, or a hybrid of both. The backlog manager deduplicates failures by stable key, tracks state transitions, detects regressions, and computes priority scores.

Two operating modes integrate with the development cycle: full mode runs the complete pipeline during META loops (periodic deep reviews), producing trend reports and feeding P1 items into the next cycle plan. Gate mode runs a lightweight pass/fail check after each commit, flagging new failures and regressions without blocking.

An anti-flap state machine governs item lifecycle (ACTIVE, TENTATIVE, RESOLVED, STALE), requiring multiple consecutive passes before marking an item resolved. This prevents the fix-regress-fix oscillation that plagues naive tracking. Cascade detection triggers an emergency protocol when multiple P1 items are simultaneously active, signaling a systemic issue rather than isolated failures.

The pipeline produces a quality report with trend calculation over a sliding window, giving the agent a clear signal: IMPROVING, STABLE, or DEGRADING.

```
{TEST_RUNNER} -> {FAILURE_ANALYZER} -> {BACKLOG_MANAGER}
```

## Implementation

### Structure

```
QUALITY_PIPELINE
├── test_runner: component       # Executes tests, produces raw results
├── failure_analyzer: component  # Classifies failures, scores severity
├── backlog_manager: component   # Deduplicates, prioritizes, tracks state
├── mode: FULL | GATE
├── last_run: timestamp
└── metrics: QUALITY_METRICS
```

```
QUALITY_METRICS
├── total_tests: int
├── passed: int
├── failed: int
├── pass_rate: float
├── active_failures: int         # ACTIVE state items
├── tentative_fixes: int         # TENTATIVE state items
├── resolved_this_cycle: int
├── regression_count: int        # RESOLVED -> ACTIVE transitions
├── p1_count: int
├── p2_count: int
└── trend: IMPROVING | STABLE | DEGRADING
```

### Three-Stage Pipeline

#### Stage 1: Test Runner (`{TEST_RUNNER}`)

Executes the test/verification suite and produces structured results:

```
INPUT:  {TEST_COMMAND}
OUTPUT: list[TEST_RESULT]

TEST_RESULT
├── test_id: string             # Stable identifier
├── name: string
├── status: PASS | FAIL | ERROR | SKIP
├── duration: float
├── error_message: string       # if FAIL/ERROR
├── error_detail: string        # stack trace, diff, etc.
├── category: string            # test suite grouping
└── metadata: object            # framework-specific data
```

Requirements:
- Test IDs must be stable across runs (same test = same ID)
- Both FAIL and ERROR are captured (assertion failure vs crash)
- SKIP is tracked but not counted as failure
- Duration captured for performance regression detection

#### Stage 2: Failure Analyzer (`{FAILURE_ANALYZER}`)

Classifies each failure and scores it:

```
INPUT:  list[TEST_RESULT] where status = FAIL | ERROR
OUTPUT: list[ANALYZED_FAILURE]

ANALYZED_FAILURE
├── test_id: string
├── category: string            # from {FAILURE_TAXONOMY} (F1-F17 or agent-specific)
├── severity: int (1-5)
├── root_cause_hypothesis: string
├── suggested_fix: string
├── north_star_impact: float    # multiplier from taxonomy
├── dedup_key: string           # {CATEGORY}:{TEST_ID}
└── is_regression: bool         # was this previously RESOLVED?
```

**Classification approach:**

| Method | When to Use | Configuration |
|--------|-------------|---------------|
| Rule-based | Error messages match known patterns | `{CLASSIFICATION_RULES}` mapping patterns -> categories |
| LLM-based | Complex failures requiring context | `{LLM_EVALUATOR_PROMPT}` with multi-dimension scoring |
| Hybrid | Default recommendation | Rules first, LLM for unmatched |

**LLM evaluator** (when configured):
- Scores failures across `{EVAL_DIMENSIONS}` dimensions (e.g., accuracy, completeness, consistency)
- Each dimension scored `{SCORE_MIN}` to `{SCORE_MAX}`
- Aggregate score determines severity
- Evaluator prompt is versioned and tracked as a tool artifact

#### Stage 3: Backlog Manager (`{BACKLOG_MANAGER}`)

Deduplicates, prioritizes, and tracks state of all analyzed failures:

```
INPUT:  list[ANALYZED_FAILURE] + existing BACKLOG
OUTPUT: updated BACKLOG + QUALITY_REPORT

BACKLOG
├── items: list[BACKLOG_ITEM]
├── total_active: int
├── total_tentative: int
├── total_resolved: int
└── total_stale: int

BACKLOG_ITEM
├── dedup_key: string
├── state: ACTIVE | TENTATIVE | RESOLVED | STALE  # from anti-flap state machine
├── priority: float              # from scoring formula
├── priority_level: P1 | P2 | P3
├── consecutive_passes: int
├── fix_attempts: int
├── first_seen_run: int
├── last_seen_run: int
├── last_failure: ANALYZED_FAILURE
└── history: list[{run_id, status, timestamp}]
```

Operations per run:
1. **Match** incoming failures to existing items by dedup_key
2. **Create** new items for unmatched failures (state = ACTIVE)
3. **Update** matched items (refresh last_seen_run, update failure details)
4. **Advance** items not in current failures (increment consecutive_passes if ACTIVE/TENTATIVE)
5. **Transition** states per anti-flap rules (see Validation Gates pattern)
6. **Score** all ACTIVE items using priority formula (see Incident Taxonomy pattern)
7. **Detect** regressions (RESOLVED -> ACTIVE transitions)
8. **Archive** STALE items (not seen for `{STALENESS_THRESHOLD}`+ runs)

### Two Operating Modes

| Mode | When | What It Does | Duration |
|------|------|-------------|----------|
| `--full` | META loops | Full pipeline: all tests, LLM analysis, backlog update, trend report | `{FULL_DURATION}` |
| `--gate` | Post-commit in regular loops | Quick test run, simple pass/fail, advisory only | `{GATE_DURATION}` |

**`--full` mode** (META loop integration):
```
1. Run complete test suite ({TEST_COMMAND})
2. Analyze all failures (rule-based + LLM if configured)
3. Update backlog (dedup, prioritize, state transitions)
4. Generate quality report with:
   - Pass rate trend (improving/stable/degrading)
   - P1/P2 items requiring attention
   - Regressions detected
   - Items resolved since last full run
   - Recommendations for next cycle
5. Feed P1 items into cycle plan as priority work
```

**`--gate` mode** (regular loop integration):
```
1. Run test suite ({TEST_COMMAND})
2. Compare against last known state
3. Report: new failures? regressions? pass rate change?
4. Advisory only — does NOT block commit
5. Flags for META review if:
   - {REGRESSION_NEW_FAILURES}+ new failures detected
   - Pass rate dropped by > {REGRESSION_RATE_DROP}%
   - {CASCADE_THRESHOLD}+ P1 items simultaneously active
```

### Regression Detection

Three types of regression trigger alerts:

| Type | Detection | Response |
|------|-----------|----------|
| New failure | Test ID not in backlog, status = FAIL | Create new ACTIVE item |
| Recurrence | RESOLVED item fails again within `{WATCH_WINDOW}` | Transition to ACTIVE, increment fix_attempts |
| Rate regression | Pass rate drops > `{REGRESSION_RATE_DROP}`% vs last run | Flag for META review |

**Cascade detection:**
```
IF {CASCADE_THRESHOLD}+ P1 items are simultaneously ACTIVE
THEN trigger emergency protocol:
  1. Pause regular loop execution
  2. Run full diagnostic ({TEST_COMMAND} with verbose output)
  3. Identify common root cause (if any)
  4. Escalate to owner with full report
  5. Do not auto-fix — wait for guidance
```

### Quality Report Format

```
QUALITY_REPORT
├── run_id: string
├── timestamp: datetime
├── mode: FULL | GATE
├── metrics: QUALITY_METRICS
├── new_failures: list[ANALYZED_FAILURE]
├── regressions: list[BACKLOG_ITEM]
├── resolved: list[BACKLOG_ITEM]
├── top_p1: list[BACKLOG_ITEM]     # sorted by priority score
├── trend: IMPROVING | STABLE | DEGRADING
└── recommendations: list[string]
```

Trend calculation:
```
IF pass_rate increased by > {TREND_IMPROVEMENT}% over last {TREND_WINDOW} runs -> IMPROVING
IF pass_rate decreased by > {TREND_DEGRADATION}% over last {TREND_WINDOW} runs -> DEGRADING
ELSE -> STABLE
```

### Configuration

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `{TEST_COMMAND}` | Command to run test suite | "pytest tests/ -v" | yes |
| `{TEST_RUNNER}` | Test runner component name | "pytest_runner" | yes |
| `{FAILURE_ANALYZER}` | Analyzer component name | "hybrid_analyzer" | yes |
| `{BACKLOG_MANAGER}` | Backlog component name | "json_backlog" | yes |
| `{FAILURE_TAXONOMY}` | Reference to taxonomy pattern | "F1-F17" | yes |
| `{CLASSIFICATION_RULES}` | Pattern -> category mapping | Config file path | yes |
| `{LLM_EVALUATOR_PROMPT}` | Prompt for LLM-based analysis | Prompt template path | no |
| `{EVAL_DIMENSIONS}` | LLM evaluation dimensions | ["accuracy", "completeness"] | no |
| `{SCORE_MIN}` | Min score per dimension | 1 | no |
| `{SCORE_MAX}` | Max score per dimension | 5 | no |
| `{FULL_DURATION}` | Expected duration of full run | "5-15 min" | no |
| `{GATE_DURATION}` | Expected duration of gate run | "1-3 min" | no |
| `{REGRESSION_NEW_FAILURES}` | New failures to flag for META | 3 | yes |
| `{REGRESSION_RATE_DROP}` | Pass rate drop % to flag | 20 | yes |
| `{CASCADE_THRESHOLD}` | Simultaneous P1 for emergency | 3 | yes |
| `{STALENESS_THRESHOLD}` | Runs without observation -> stale | 10 | yes |
| `{WATCH_WINDOW}` | Runs to watch after RESOLVED | 5 | yes |
| `{TREND_IMPROVEMENT}` | % increase to classify IMPROVING | 5 | no |
| `{TREND_DEGRADATION}` | % decrease to classify DEGRADING | 5 | no |
| `{TREND_WINDOW}` | Runs to calculate trend over | 10 | no |

### Decision Rules

- **Full vs gate**: Always run `--full` during META loops. Always run `--gate` after commits in regular loops. Never skip both in the same cycle.
- **LLM vs rules**: Use rules for failures with clear error patterns (assertion messages, known error codes). Use LLM for failures requiring context understanding (output quality, behavioral correctness). Default to hybrid.
- **P1 into cycle plan**: All P1 items from `--full` run must appear in the next cycle plan. P2 items are discretionary. P3 items wait for IMPROVE loops.
- **Cascade response**: If cascade is detected, the agent stops all regular work and focuses exclusively on diagnosis. This is not optional.
- **Trend interpretation**: DEGRADING trend for `{TREND_WINDOW}`+ runs = systemic issue. Don't fix individual failures -- investigate root cause.

<!-- REFERENCE: load on adoption -->
## Adaptation Guide

### Prerequisites
- A test suite with stable test IDs (same test = same ID across runs)
- An incident taxonomy (for failure classification) -- see Incident Taxonomy pattern
- An anti-flap state machine (for regression tracking) -- see Validation Gates pattern
- A loop protocol with META and regular loops -- see Autonomous Loop Protocol
- File system for backlog persistence

### Steps to Adopt
1. **Implement test runner wrapper** that captures structured TEST_RESULT objects from your test framework
2. **Define classification rules** mapping error patterns to taxonomy categories (start with 5-10 rules)
3. **Implement backlog manager** with JSON file persistence (one file, overwritten per run)
4. **Wire `--gate` mode** into regular loop pipeline (after commit step)
5. **Wire `--full` mode** into META loop protocol (step 2: quality system full run)
6. **Add regression detection** (new failures + recurrence + rate drop)
7. **Add trend calculation** over sliding window
8. **Optionally add LLM evaluator** for complex failure analysis
9. **Add circuit breakers** for cascade detection
10. **Write unit tests** for backlog state transitions (reuse anti-flap tests from Validation Gates)

### What to Customize
- Test command and framework (pytest, jest, go test, custom)
- Classification rules (domain-specific error patterns)
- LLM evaluator prompt and dimensions (if used)
- Threshold values for regression detection
- Report format and content
- Trend window size
- Cascade threshold

### What NOT to Change
- Three-stage pipeline structure (runner -> analyzer -> manager)
- Two operating modes (full + gate)
- Full run during META, gate after commit (integration points with loop protocol)
- Deduplication by stable key (not by error message text)
- Anti-flap state machine for tracking (don't mark items resolved after single pass)
- Cascade detection as emergency protocol (don't make it advisory)
- P1 items must enter cycle plan (not optional backlog)
- Trend as sliding window calculation (not point-in-time comparison)

<!-- HISTORY: load for audit -->
## Origin

### voic-experiment
- **Findings:** [16] CLAQS Quality System Design, [28] Three-Stage Pipeline, [29] Full vs Gate Modes, [30] Regression Detection, [31] LLM Evaluator Integration
- **Discovered through:** 226 loops of development where manual quality tracking broke down after loop 50. The three-stage pipeline emerged from separating concerns: running tests, understanding failures, and managing work. Two modes (full/gate) emerged from the observation that full analysis was too expensive for every loop but too valuable to skip entirely. LLM evaluator added when rule-based classification couldn't handle nuanced quality issues (e.g., "output is technically correct but misleading"). Regression detection added after observing that 15% of "fixed" items reappeared within 5 loops.
- **Evidence:** Pipeline processed 226 loops of test results. Automated prioritization reduced time-to-fix for P1 items. Gate mode caught regressions before they accumulated. LLM evaluator correctly classified failures that rules missed in 73% of cases. Anti-flap integration reduced regression oscillation to < 2% of loops.

## Related Patterns
- [Autonomous Loop Protocol](../patterns/loop_protocol.md) -- quality system integrates with META (full) and regular (gate) loops
- [Incident Taxonomy](../patterns/incident_taxonomy.md) -- provides failure categories (F1-F17) and priority scoring formula
- [Validation Gates](../patterns/validation_gates.md) -- anti-flap state machine tracks item stability; circuit breakers prevent runaway
- [Confidence & Severity Model](../methodologies/confidence_severity.md) -- PARTIAL OVERLAP: both address confidence scoring but through different mechanisms. This pattern uses pipeline-based quality measurement. Confidence & Severity uses a scoring model with decay. They complement each other: this pattern generates severity scores, Confidence & Severity tracks their evolution over time
- [Three-Tier Backlog Management](../methodologies/backlog_management.md) -- quality pipeline is the primary source of auto-backlog items
- [Adversarial Reflection](../patterns/adversarial_reflection.md) -- adversarial review improves quality loop's failure classification
- [Append-Only Audit Trail](../patterns/append_only_audit.md) -- quality tracking relies on preserved audit records
- [Analysis-Review-Merge Pipeline](../methodologies/analysis_review_merge.md) -- quality issues from merge feed back into quality loop
- [Invariant-Based Testing](../methodologies/invariant_testing.md) -- invariants validate quality pipeline outputs
- [Maturity Metrics](../methodologies/maturity_metrics.md) -- quality loop generates the run data that feeds sigma metric
- [Repository-as-Product (RaP)](../methodologies/rap_methodology.md) -- reflect phase feeds quality improvement loop
