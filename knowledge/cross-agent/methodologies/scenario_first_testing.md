---
id: cross-scenario-first-testing
title: Scenario-First Testing
type: methodology
concern: [testing]
mechanism: [pipeline, registry]
scope: per-cycle
lifecycle: [act, reflect]
origin: harvest/voic-experiment
origin_findings: [19, 26, 27, 52, 20]
maturity: draft
domain: cross-agent/methodologies
confidence: observed
basis: "distilled from voic-experiment harvest, 65 findings from voice agent sessions"
---

# Scenario-First Testing

<!-- CORE: load always -->
## Problem

End-to-end tests for complex systems are typically added late in the development cycle, written reactively after bugs ship, and structured ad-hoc by whoever writes them. Each test script follows its own conventions, making the suite hard to review, extend, or trust.

Nondeterministic inputs compound the problem: tests that generate random data at runtime produce different results on each run, making failures non-reproducible. When a test fails, it is unclear whether the system broke or the input changed. Flaky tests erode trust in the entire suite, and teams begin ignoring failures.

Without a unified quality evaluation framework, pass/fail is the only signal. This binary view misses gradual quality degradation -- output that is technically correct but increasingly poor along dimensions like completeness, latency, or coherence. There is no gate mechanism to prevent regressions from reaching production, so quality issues are discovered by users rather than by the test infrastructure.

## Solution

Scenarios are defined as declarative YAML specifications and written before the feature or fix they validate. This scenario-first discipline forces developers to articulate expected behavior upfront, making test definitions reviewable and composable artifacts rather than opaque scripts.

A headless client executes scenarios programmatically against the real system endpoint without any UI rendering, making execution fast, stable, and CI-compatible. A batch runner orchestrates multiple scenarios with controlled concurrency and atomic result writes, ensuring that partial failures do not corrupt the result set.

Each scenario execution produces a structured session record scored across multiple quality dimensions (e.g., correctness, latency, completeness). Scoring methods include rule-based checks for deterministic properties, metric thresholds for performance SLOs, and optionally LLM-evaluated grading for subjective quality dimensions.

A quality gate applies binary pass/fail criteria to the aggregate batch result: minimum pass rate, minimum average quality score, zero critical anomalies, and per-scenario SLO compliance. The gate blocks releases that fail to meet standards, ensuring that regressions are caught by infrastructure rather than by users.

## Implementation

### Structure

```
SCENARIO_REGISTRY
├── scenarios/
│   ├── {SCENARIO_ID}.yaml       # scenario definitions
│   └── ...
├── inputs/
│   ├── {SCENARIO_ID}/           # pre-generated input data per scenario
│   └── ...
├── results/
│   ├── {RUN_ID}/
│   │   ├── summary.json         # aggregate results for this run
│   │   └── {SCENARIO_ID}.json   # per-scenario structured result
│   └── ...
└── config/
    ├── quality_dimensions.yaml   # scoring rubric
    └── gate_thresholds.yaml      # pass/fail criteria

SCENARIO (YAML)
├── id: string
├── name: string
├── description: string
├── config: object                # scenario-specific configuration
├── goal: string                  # what this scenario validates
├── slo_targets: SLO_TARGETS      # performance and quality thresholds
├── inputs: list[INPUT]           # pre-generated deterministic inputs
└── steps: list[STEP]             # ordered test steps

SLO_TARGETS
├── max_latency_ms: int           # P95 latency target for the scenario
├── min_quality_score: float      # minimum acceptable quality (0.0 - 1.0)
└── custom: dict                  # domain-specific SLO targets

STEP
├── action: string                # what to do (e.g., "submit_request", "wait_for_result")
├── input_ref: string | null      # reference to pre-generated input
├── expected: EXPECTED | null     # expected outcome (null = no assertion, just capture)
├── timeout_ms: int               # max wait for this step
└── capture: list[string]         # fields to capture in result record

EXPECTED
├── status: string                # expected status code or state
├── contains: list[string]        # substrings that must be present in output
├── schema: object | null         # JSON schema the output must match
└── custom_checks: list[string]   # named custom validation functions

SESSION_RECORD
├── scenario_id: string
├── run_id: string
├── timestamp: datetime
├── duration_ms: int
├── steps: list[STEP_RESULT]
├── quality_scores: dict[dimension → float]
├── anomalies: list[ANOMALY]
├── overall_pass: boolean
└── metadata: object

ANOMALY
├── type: string                  # classification of what went wrong
├── severity: LOW | MEDIUM | HIGH | CRITICAL
├── step_index: int               # which step triggered the anomaly
├── message: string
└── context: object               # relevant data for debugging
```

### Scenario Lifecycle

```
DEFINE → GENERATE_INPUTS → EXECUTE → EVALUATE → GATE → REPORT
```

| Phase | Actor | Action |
|-------|-------|--------|
| DEFINE | Developer/Agent | Write scenario YAML before implementing feature or fix |
| GENERATE_INPUTS | {INPUT_GENERATOR} | Create deterministic inputs for reproducibility |
| EXECUTE | {HEADLESS_CLIENT} | Run scenario steps against the system under test |
| EVALUATE | {EVALUATOR} | Score results across quality dimensions |
| GATE | {GATE_CHECKER} | Apply pass/fail thresholds to aggregate results |
| REPORT | {REPORTER} | Generate human-readable summary with drill-down |

### Input Pre-Generation

All test inputs are generated ahead of execution and stored as static files:

```yaml
# inputs/{SCENARIO_ID}/turn_001.yaml
input_id: "turn_001"
data:
  field_1: "{DETERMINISTIC_VALUE_1}"
  field_2: "{DETERMINISTIC_VALUE_2}"
  field_3: "{DETERMINISTIC_VALUE_3}"
generated_at: "2025-01-15T10:00:00Z"
generator_version: "1.0"
```

Why pre-generate:
- **Reproducibility:** Same inputs every run, failures are deterministic and debuggable
- **Isolation:** Input generation bugs do not mask system bugs
- **Versioning:** Inputs are tracked in version control alongside scenarios
- **Speed:** No generation overhead during execution

### Headless Client

A programmatic client that exercises the system without a graphical interface:

```
{HEADLESS_CLIENT}
├── connect(endpoint: string)      # establish connection to system under test
├── execute_step(step: STEP)       # perform one scenario step
├── capture_result(step: STEP)     # collect output and metrics
├── disconnect()                   # clean up connection
└── health_check()                 # verify system is ready before running
```

Key properties:
- Connects to the same endpoint a real user would (not mock endpoints)
- Captures structured results per step (latency, output, errors)
- No UI rendering — faster, more stable, CI-compatible
- Timeout enforcement per step

### Batch Runner

Orchestrates multiple scenarios with controlled concurrency:

```
{TEST_RUNNER}
├── load_scenarios(path: string)                  # load all YAML scenarios
├── validate_scenarios()                          # schema check before execution
├── execute_batch(concurrency: int)               # run scenarios in parallel
├── collect_results() → list[SESSION_RECORD]      # gather all results
└── write_results(path: string)                   # atomic write per scenario
```

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `{MAX_CONCURRENCY}` | Maximum parallel scenario executions | `4` | yes |
| `{RETRY_ON_INFRA_ERROR}` | Retry count for infrastructure failures (not test failures) | `2` | yes |
| `{RESULT_WRITE_MODE}` | Result write strategy — each result written as complete file, no partial writes | `atomic` | yes |

### Quality Evaluation

Multi-dimensional scoring applied to each `SESSION_RECORD`:

```yaml
# config/quality_dimensions.yaml
dimensions:
  - name: "{DIMENSION_1}"
    weight: "{WEIGHT_1}"
    scoring: "{SCORING_METHOD}"       # rule-based | llm-evaluated | metric-threshold
    description: "..."

  - name: "{DIMENSION_2}"
    weight: "{WEIGHT_2}"
    scoring: "{SCORING_METHOD}"
    description: "..."

  - name: "{DIMENSION_3}"
    weight: "{WEIGHT_3}"
    scoring: "{SCORING_METHOD}"
    description: "..."
```

Scoring methods:
- **rule-based:** Deterministic checks (status codes, schema validation, substring matching)
- **llm-evaluated:** LLM grades output against rubric (for subjective quality dimensions)
- **metric-threshold:** Numerical metric compared against SLO target

Weighted aggregate:

```
overall_quality = sum(dimension_score × dimension_weight) / sum(weights)
```

### Quality Gate

Binary pass/fail decision applied to a batch run:

```
GATE CONDITIONS (ALL must pass):
  1. Pass rate:         passed_scenarios / total_scenarios >= {MIN_PASS_RATE}
  2. Average quality:   mean(overall_quality) >= {MIN_AVG_QUALITY}
  3. Critical anomalies: count(anomalies where severity == CRITICAL) == 0
  4. SLO compliance:    each scenario meets its slo_targets
```

| Condition | Threshold | On Failure |
|-----------|-----------|------------|
| Pass rate | >= `{MIN_PASS_RATE}` (e.g., 90%) | BLOCK release |
| Average quality | >= `{MIN_AVG_QUALITY}` (e.g., 0.8) | BLOCK release |
| Critical anomalies | == 0 | BLOCK release |
| SLO compliance | Per-scenario targets | WARN (configurable to BLOCK) |

### Canonical Data Models

All tools in the testing pipeline share canonical data models — one definition, used everywhere:

| Model | Used By | Purpose |
|-------|---------|---------|
| `SESSION_RECORD` | {HEADLESS_CLIENT}, {EVALUATOR}, {REPORTER}, {GATE_CHECKER} | Complete test result per scenario |
| `ANOMALY` | {HEADLESS_CLIENT}, {EVALUATOR} | Structured representation of a detected problem |
| `SCENARIO` | {TEST_RUNNER}, {HEADLESS_CLIENT}, {INPUT_GENERATOR} | Test specification |

This prevents data translation bugs between pipeline stages. One schema, shared by all.

### Configuration

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `{SCENARIO_ID}` | Unique scenario identifier | "checkout-happy-path" | yes |
| `{INPUT_GENERATOR}` | Tool that creates deterministic inputs | "input_gen.py" | yes |
| `{HEADLESS_CLIENT}` | Programmatic client for system under test | "headless_client.py" | yes |
| `{TEST_RUNNER}` | Batch orchestrator | "run_scenarios.py" | yes |
| `{EVALUATOR}` | Quality scoring engine | "evaluate.py" | yes |
| `{GATE_CHECKER}` | Pass/fail gate logic | "gate_check.py" | yes |
| `{REPORTER}` | Human-readable report generator | "report.py" | no |
| `{MAX_CONCURRENCY}` | Max parallel scenario executions | 4 | yes |
| `{MIN_PASS_RATE}` | Minimum scenario pass rate for gate | 0.90 | yes |
| `{MIN_AVG_QUALITY}` | Minimum average quality score for gate | 0.80 | yes |
| `{DIMENSION_N}` | Name of a quality dimension | "correctness" | yes |
| `{WEIGHT_N}` | Weight of a quality dimension | 0.4 | yes |
| `{SCORING_METHOD}` | How dimension is scored | "rule-based" | yes |

### Decision Rules

| Situation | Action |
|-----------|--------|
| New feature requested | Write scenario YAML first, then implement |
| Bug reported | Write regression scenario first, then fix |
| Scenario fails after fix | Fix is incomplete — do not merge |
| Flaky test (passes/fails nondeterministically) | Check input determinism first — are inputs pre-generated? |
| Quality gate fails on pass rate | Analyze failed scenarios, fix, re-run full batch |
| Quality gate fails on average quality | Review dimension scores, identify weakest dimension |
| Critical anomaly detected | Investigate immediately — blocks all releases |
| New quality dimension needed | Add to quality_dimensions.yaml, backfill scores on existing scenarios |

<!-- REFERENCE: load on adoption -->
## Adaptation Guide

### Prerequisites
- A system under test with a programmatic interface (API, CLI, or protocol endpoint)
- YAML parsing capability in the test toolchain
- A CI/CD pipeline where the quality gate can block deployments
- At least 3 quality dimensions defined for your domain

### Steps to Adopt
1. Define 3-5 quality dimensions relevant to your domain (e.g., correctness, latency, completeness)
2. Write the SCENARIO YAML schema — start with `id`, `name`, `goal`, `steps`, `slo_targets`
3. Implement the {INPUT_GENERATOR} — create deterministic inputs for your first 3 scenarios
4. Implement the {HEADLESS_CLIENT} — a minimal programmatic client for your system
5. Write your first 3 scenarios covering the most critical user paths
6. Implement the {TEST_RUNNER} — sequential first, add concurrency later
7. Implement rule-based scoring for your quality dimensions
8. Set quality gate thresholds conservatively (80% pass rate, 0.7 avg quality) and tighten over time
9. Integrate the gate into CI/CD — failing gate blocks merge/deploy
10. Add LLM-evaluated scoring for subjective dimensions (optional, high-value)

### What to Customize
- Quality dimensions and weights (entirely domain-specific)
- Scenario YAML fields (add domain-specific config, remove unused fields)
- Scoring methods per dimension (rule-based vs. LLM-evaluated vs. metric-threshold)
- Gate thresholds (start lenient, tighten as test suite matures)
- Concurrency limits (depends on system under test capacity)
- Input generation strategy (random seed, template-based, real data anonymized)

### What NOT to Change
- Scenario-first discipline — writing tests after implementation defeats the purpose
- Declarative YAML format — imperative test scripts become unmaintainable at scale
- Input pre-generation — runtime-generated inputs make failures non-reproducible
- Canonical data models — duplicating `SESSION_RECORD` across tools creates translation bugs
- Quality gate as a binary blocker — advisory-only gates get ignored
- Multi-dimensional scoring — single pass/fail misses nuanced quality degradation
- Atomic result writes — partial writes corrupt the result set

<!-- HISTORY: load for audit -->
## Origin

### voic-experiment
- **Findings:** [19] Scenario YAML format and scenario-first discipline, [26] Headless client for automated E2E testing, [27] Batch runner with concurrent execution and atomic result writes, [52] LLM evaluator for multi-dimensional quality scoring, [20] Canonical data models as single source of truth for all pipeline tools
- **Discovered through:** Building an E2E test suite for a multi-stage pipeline. Initial tests were ad-hoc scripts with hardcoded inputs — flaky, hard to review, impossible to extend. Moving to declarative YAML scenarios made tests reviewable and composable. Pre-generated inputs eliminated flakiness from nondeterministic input generation. The quality gate was added after a regression shipped to production that passed basic assertions but degraded subjective quality. Multi-dimensional scoring with LLM evaluation caught quality issues that rule-based checks missed.
- **Evidence:** Scenario-first discipline caught design issues before implementation. Pre-generated inputs reduced flaky test rate to near zero. Quality gate blocked 3 regressions that would have shipped. Multi-dimensional scoring identified gradual quality degradation invisible to pass/fail assertions.

## Related Patterns
- [Validation Gates](../patterns/validation_gates.md) — quality gate is a specialized validation gate for test results
- [Provider Resilience](../patterns/provider_resilience.md) — test infrastructure itself benefits from resilient provider access
- [Data Quality Framework](../methodologies/data_quality.md) — input pre-generation is a data quality practice applied to test data
- [Monitoring Principles](../best_practices/monitoring_principles.md) — quality dimensions are monitoring metrics for system correctness
- [Artifact-Centric Interface](../patterns/artifact_centric_interface.md) — test results stored as structured files follow Layer 1
- [Adversarial Reflection](../patterns/adversarial_reflection.md) -- adversarial review strengthens scenario quality evaluation
- [Analysis-Review-Merge Pipeline](../methodologies/analysis_review_merge.md) -- end-to-end scenarios test the full three-phase pipeline
- [Context Budget Management](../methodologies/context_budget.md) -- pre-generated test data avoids context-heavy exploration
- [Invariant-Based Testing](../methodologies/invariant_testing.md) -- invariants complement scenario assertions for property-based quality checks
- [Maturity Metrics](../methodologies/maturity_metrics.md) -- scenarios are a source of defined invariants feeding iota metric
