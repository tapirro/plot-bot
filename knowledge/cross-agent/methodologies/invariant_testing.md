---
id: cross-invariant-testing
title: Invariant-Based Testing
type: methodology
concern: [testing, validation-gating]
mechanism: [gate-pyramid]
scope: per-item
lifecycle: [detect, classify]
origin: harvest/spec-creator
origin_findings: [19, 84]
maturity: draft
domain: cross-agent/methodologies
confidence: observed
basis: "distilled from spec-creator harvest, 104 findings across 16+ agent sessions"
---

# Invariant-Based Testing

<!-- CORE: load always -->
## Problem

Semantic processes -- LLM-driven extraction, content generation, knowledge synthesis -- produce outputs where no single correct answer exists. Two valid extractions from the same source may differ in wording, ordering, granularity, and emphasis while both being correct.

Traditional testing compares output against a golden reference, but this approach fails for non-deterministic processes. Valid reformulations get flagged as errors, while hallucinated content that happens to match the golden reference passes undetected. The result is a test suite that penalizes diversity in correct outputs and rewards memorization of the reference.

Without an alternative testing strategy, quality assessment falls back to subjective human review or is skipped entirely. Teams cannot distinguish between runs that produce different-but-valid output and runs that produce genuinely degraded output. There is no formal definition of "correct" that can be checked automatically, so regressions go unnoticed until downstream consumers report problems.

## Solution

Replace golden-answer comparison with property-based correctness. Instead of asserting what the output should be, assert what properties all valid outputs must satisfy. These properties are organized into four orthogonal invariant classes: structural (schema conformance, referential integrity), coverage (source linkage, bidirectional references), groundedness (evidence backing, no hallucination), and stability (count convergence, content drift limits across runs).

Every discovered bug is formulated as a new invariant that would have caught it, and added to the suite. The invariant set grows monotonically -- it never shrinks. Obsolete invariants are marked SKIP with a reason but never deleted, preserving the full history of quality lessons learned.

A minimum expected invariant count (I_exp) is computed from the number of artifact types and model relations, providing a quantitative target for testing investment. The ratio of actual to expected invariants serves as a maturity signal: systems below I_exp consistently have undetected quality issues.

After any refactoring of instructions, scripts, or pipeline configuration, a mandatory smoke-test runs the full invariant suite on the last known-good input. This protocol was established after a phase rename broke the pipeline for multiple days because no invariant check was run post-refactoring.

## Implementation

### Structure

```
INVARIANT_CLASSES
├── structural (binary, fully automated)
│   ├── schema_conformance: "every artifact matches its declared schema"
│   ├── referential_integrity: "every ID reference resolves to an existing artifact"
│   ├── uniqueness: "no duplicate IDs across the artifact set"
│   ├── required_fields: "no required field is empty or null"
│   └── format_compliance: "dates, enums, and typed fields match their declared format"
├── coverage (threshold-based, fully automated)
│   ├── source_linkage: "{COVERAGE_THRESHOLD}% of sources linked to at least one artifact"
│   ├── bidirectional_refs: "every forward reference has a corresponding back-reference"
│   ├── type_distribution: "no artifact type has 0 instances (if sources warrant it)"
│   └── field_completeness: "{FIELD_COMPLETENESS_THRESHOLD}% of optional fields populated"
├── groundedness (semi-automated, requires source access)
│   ├── evidence_backing: "every claim references a source (SRC-* or equivalent)"
│   ├── no_hallucination: "no field contains data not traceable to input sources"
│   ├── unk_tracking: "missing info recorded as UNK record, never guessed"
│   └── quote_accuracy: "direct quotes match source text (when verifiable)"
└── stability (cross-run, automated with diff tooling)
    ├── count_convergence: "artifact counts deviate <= {MAX_COUNT_DEVIATION}% between runs"
    ├── id_stability: "existing artifact IDs preserved across runs (no re-numbering)"
    ├── content_drift: "existing artifact content changes <= {DRIFT_THRESHOLD}% between runs"
    └── type_ratio_stability: "artifact type proportions deviate <= {TYPE_RATIO_DEVIATION}%"
```

```
INVARIANT_SUITE
├── invariants: list[INVARIANT]
├── I_exp: int                       # expected minimum count
├── I_actual: int                    # current count
├── coverage_ratio: float            # I_actual / I_exp
└── last_run: datetime

INVARIANT
├── id: string                       # e.g., "INV-STRUCT-001"
├── class: structural | coverage | groundedness | stability
├── description: string              # human-readable property statement
├── check: automated | semi-automated | manual
├── added_after: string              # bug/incident that prompted this invariant
├── severity: BLOCK | WARN           # does failure block the pipeline?
└── last_result: PASS | FAIL | SKIP
```

### Expected Invariant Count

The minimum number of invariants a mature system should define:

```
I_exp = ({ARTIFACT_TYPES} × 3) + ({MODEL_RELATIONS} × 2) + 5

Where:
  {ARTIFACT_TYPES} × 3  = at least 3 structural checks per artifact type
                           (schema, required fields, format)
  {MODEL_RELATIONS} × 2 = at least 2 checks per relation
                           (forward ref exists, back-ref exists)
  + 5                    = baseline invariants
                           (no duplicate IDs, source coverage, UNK tracking,
                            count convergence, id stability)
```

### Monotonic Growth Rule

```
INVARIANT_GROWTH_POLICY
├── trigger: every discovered bug or quality incident
├── action: formulate the missing property as a new invariant
├── add_to: the appropriate class (structural/coverage/groundedness/stability)
├── never: remove an existing invariant
│   ├── if obsolete: mark as SKIP with reason, keep in suite
│   └── if wrong: fix the invariant definition, do not delete
└── result: invariant count is non-decreasing over time
```

### Smoke-Test Protocol

After any refactoring of instructions, scripts, or pipeline configuration:

```
SMOKE_TEST_AFTER_REFACTORING
├── trigger: any change to agent instructions, scripts, schemas, or pipeline config
├── action: run the FULL invariant suite on the last known-good input
├── pass: all invariants that passed before still pass
├── fail: refactoring introduced a regression -- fix before declaring success
├── skip: NEVER skip this step
└── evidence: "failed refactoring without smoke-test caused multi-day regression"
    (spec-creator incident: review phase was skipped after instruction rename
     because smoke-test was not run)
```

### Configuration

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `{ARTIFACT_TYPES}` | Number of distinct artifact types | 7 | yes |
| `{MODEL_RELATIONS}` | Number of defined relations between types | 12 | yes |
| `{COVERAGE_THRESHOLD}` | Minimum % of sources that must link to artifacts | 80 | yes |
| `{FIELD_COMPLETENESS_THRESHOLD}` | Minimum % of optional fields populated | 60 | no |
| `{MAX_COUNT_DEVIATION}` | Maximum allowed % deviation in artifact counts between runs | 30 | yes |
| `{DRIFT_THRESHOLD}` | Maximum allowed % content change in existing artifacts between runs | 20 | yes |
| `{TYPE_RATIO_DEVIATION}` | Maximum allowed % shift in artifact type proportions | 15 | no |
| `{BLOCK_ON_GROUNDEDNESS_FAIL}` | Whether groundedness failures block pipeline | true | yes |

### Decision Rules

| Situation | Action |
|-----------|--------|
| New bug discovered | Formulate the missing property as a new invariant; add to suite |
| Structural invariant fails | BLOCK -- schema/reference errors propagate downstream |
| Coverage invariant fails | WARN if above `{COVERAGE_THRESHOLD}` - 10%; BLOCK if below |
| Groundedness invariant fails | BLOCK if `{BLOCK_ON_GROUNDEDNESS_FAIL}`; investigate hallucination source |
| Stability invariant fails on first comparison | Establish baseline (need 2+ runs); WARN only |
| Stability invariant fails after baseline | Investigate nondeterminism; check prompt changes, model updates, input drift |
| I_actual < I_exp / 2 | Testing investment critically low; add invariants before new features |
| Refactoring planned | Run full suite before AND after; diff results |
| Invariant seems wrong | Fix the definition; do NOT delete the invariant |
| All invariants pass but output looks wrong | Missing invariant class; add new property to suite |

<!-- REFERENCE: load on adoption -->
## Adaptation Guide

### Prerequisites
- Defined artifact types with schemas (for structural invariants)
- At least 2 completed pipeline runs (for stability invariants)
- Source materials accessible for spot-checking (for groundedness invariants)
- A validation runner that can execute checks and return structured results

### Steps to Adopt
1. Enumerate your artifact types and relations; compute I_exp
2. Implement structural invariants first (schema conformance, referential integrity, uniqueness) -- highest ROI, fully automated
3. Add coverage invariants (source linkage, bidirectional refs) -- catches extraction gaps
4. Run the pipeline twice on the same input; implement stability invariants from the diff
5. Add groundedness invariants (evidence backing, UNK tracking) -- catches hallucination
6. Set up the smoke-test protocol: full suite run after every refactoring
7. Establish the monotonic growth rule: every bug becomes a new invariant
8. Track I_actual / I_exp as a maturity signal

### What to Customize
- Specific invariants per artifact type (depends on your schemas and domain)
- Thresholds for coverage, drift, and deviation (start lenient, tighten as system matures)
- Which groundedness checks are automated vs. semi-automated (depends on source accessibility)
- Severity levels per invariant (BLOCK vs. WARN depends on pipeline criticality)
- Additional invariant classes for domain-specific properties

### What NOT to Change
- Four invariant classes (structural, coverage, groundedness, stability) -- they cover orthogonal quality dimensions
- Monotonic growth rule -- invariants never decrease, only grow or get marked SKIP
- Smoke-test after refactoring -- skipping this caused the worst regressions observed
- I_exp formula as a minimum target -- systems below this threshold have blind spots
- Bug-to-invariant conversion -- every quality incident must produce a lasting check
- Structural invariants are always BLOCK -- schema violations cascade to every downstream consumer

<!-- HISTORY: load for audit -->
## Origin

### spec-creator
- **Findings:** [19] Invariant-Based Testing for Non-Deterministic Outputs, [84] Validate Smoke-Test After Refactoring
- **Discovered through:** Building a specification extraction pipeline that processes source code into structured artifacts. Early attempts used golden-output comparison, but valid extractions from the same source differed in wording, ordering, and granularity. The insight: all valid outputs shared structural properties (schema conformance, referential integrity, source coverage) even when content varied. Four invariant classes emerged from categorizing 20+ discovered bugs. The I_exp formula was derived empirically: systems with fewer invariants than I_exp consistently had undetected quality issues. The smoke-test protocol was added after a phase rename (from "artifact_review" to "review" + "merge") broke the pipeline for multiple days because no invariant suite was run post-refactoring.
- **Evidence:** Across 16+ extraction runs, invariant-based testing caught issues that golden-output comparison would have missed (valid reformulations flagged as errors) and issues that golden-output would have missed entirely (hallucinated fields that looked plausible). The smoke-test gap after the phase rename incident cost an estimated 2-3 days of debugging. After the smoke-test protocol was established, zero regressions escaped refactoring.

## Related Patterns
- [Maturity Metrics](../methodologies/maturity_metrics.md) -- iota metric directly measures invariant density (I_actual / I_exp)
- [Validation Gates](../patterns/validation_gates.md) -- invariant checks can be wired into gate pyramid layers
- [Analysis-Review-Merge Pipeline](../methodologies/analysis_review_merge.md) -- invariants run after merge phase as final quality gate
- [Scenario-First Testing](../methodologies/scenario_first_testing.md) -- scenarios test end-to-end paths; invariants test output properties
- [Closed-Loop Quality System](../methodologies/closed_loop_quality.md) -- quality loop generates the bugs that become new invariants
- [Monitoring Principles](../best_practices/monitoring_principles.md) -- invariant pass/fail rates are monitoring signals
- [Cognitive Offload](../methodologies/cognitive_offload.md) -- invariant checks are mechanical work offloaded to scripts
