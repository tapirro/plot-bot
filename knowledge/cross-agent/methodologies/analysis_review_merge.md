---
id: cross-analysis-review-merge
title: Analysis-Review-Merge Pipeline
type: methodology
concern: [pipeline-control, validation-gating]
mechanism: [pipeline, gate-pyramid]
scope: per-cycle
lifecycle: [detect, classify, act]
origin: harvest/spec-creator
origin_findings: [10, 13, 46, 53]
maturity: draft
domain: cross-agent/methodologies
confidence: observed
basis: "distilled from spec-creator harvest, 104 findings across 16+ agent sessions"
---

# Analysis-Review-Merge Pipeline

<!-- CORE: load always -->
## Problem

When agents extract structured artifacts from unstructured sources, mixing discovery with artifact creation causes hallucination propagation. The agent reads a source, forms an interpretation, and immediately writes an artifact -- all in one pass. If the interpretation is wrong, the artifact enshrines the error as a structured fact that downstream consumers trust implicitly.

Findings created under context pressure are never verified against original sources. As the agent's context window fills with extracted data, it begins relying on its own summaries rather than returning to source materials. Errors compound: a misread in early extraction becomes an accepted premise for later artifacts, and by the time the pipeline completes, the provenance chain from source to artifact is untraceable.

Gaps are papered over with plausible-looking invented content rather than tracked explicitly. When a required field lacks sufficient evidence, the agent fills it with a reasonable-sounding value rather than admitting uncertainty. This hallucination is invisible in the output -- the artifact looks complete and well-formed, but contains fabricated data that may not surface as an error for days.

Context compaction intensifies these problems. When the context window overflows, the LLM's compaction summary can reduce a multi-phase pipeline description to a single instruction like "create YAML cards," causing the agent to skip entire phases without realizing it.

## Solution

The pipeline is separated into three strictly isolated phases, each with a clear mandate and a hard prohibition on the others' responsibilities.

**Analysis** reads source materials and extracts raw findings with source references, confidence scores, and explicit UNK records for insufficient evidence. It creates no artifacts -- this is the phase's core invariant. Findings are written as individual JSON files (one per source or batch) that serve as the input for the next phase.

**Review** takes each finding and verifies it against the original source material. Reviewers check that findings have evidence in the source (not hallucinated), that confidence scores are justified, and that required fields are populated or have explicit UNK records. Reviewers enrich findings by filling optional fields from source data and adding cross-references. Review also creates no artifacts.

**Merge** is the only phase that creates artifacts, and it operates exclusively on reviewed findings. A mechanical review gate (a script returning pass/fail) blocks merge until review is demonstrably complete: review state file exists, all finding clusters have "reviewer done" status, and a minimum percentage of findings have review scores. The gate checks evidence of actual work, not just infrastructure creation -- it catches the anti-pattern where an agent creates review infrastructure and marks the phase complete without running any reviewers.

A coverage audit with bounded re-analysis sits between analysis and review: if source coverage falls below a target, the pipeline returns to analysis for uncovered sources only, up to a configurable maximum number of returns. This prevents both insufficient coverage and infinite retry loops.

## Implementation

### Structure

```
THREE_PHASE_PIPELINE
├── analysis
│   ├── input: source materials ({SOURCE_FORMAT})
│   ├── output: findings/*.json (one per source or batch)
│   ├── creates_artifacts: NO (strictly forbidden)
│   ├── principle: "extract, don't create"
│   └── what_it_produces:
│       ├── raw findings with source references
│       ├── confidence scores per finding
│       ├── UNK records for insufficient evidence
│       └── coverage tracking (which sources processed)
├── review
│   ├── input: findings/*.json + original source materials
│   ├── output: review_state.json + enriched findings
│   ├── creates_artifacts: NO (strictly forbidden)
│   ├── principle: "verify every finding against its evidence"
│   └── what_it_produces:
│       ├── verification scores per finding
│       ├── enriched fields (reviewer fills gaps from sources)
│       ├── rejected findings (insufficient evidence)
│       └── review_state.json (proof that review occurred)
└── merge
    ├── input: reviewed + enriched findings
    ├── output: artifact cards, indexes, coverage reports
    ├── creates_artifacts: YES (the ONLY phase that does)
    ├── prerequisite: review gate MUST pass
    └── what_it_produces:
        ├── final artifact files ({ARTIFACT_FORMAT})
        ├── updated indexes
        ├── coverage summary
        └── traceability links (artifact → finding → source)
```

### Coverage Audit with Bounded Re-analysis

```
COVERAGE_AUDIT
├── trigger: after analysis phase completes
├── check: what percentage of sources produced findings
├── if_below_{COVERAGE_TARGET}:
│   ├── identify uncovered sources
│   ├── increment analysis_iteration counter
│   ├── if analysis_iteration <= {MAX_ANALYSIS_RETURNS}:
│   │   └── return to analysis with scope = uncovered sources ONLY
│   └── if analysis_iteration > {MAX_ANALYSIS_RETURNS}:
│       └── proceed to review; document gaps explicitly
├── state_tracking:
│   ├── analysis_iteration: int (persisted in session_state for resume)
│   └── uncovered_sources: list[string]
└── rationale: bounded retry prevents infinite loops while ensuring
    reasonable coverage; typically 2 returns is sufficient
```

### Review Gate (Mechanical Enforcement)

```
REVIEW_GATE
├── purpose: prevent merge without actual review
├── implementation: script ({GATE_SCRIPT}) that returns exit 0 or exit 1
├── checks (ALL must pass):
│   ├── review_state.json exists
│   ├── all finding clusters have "reviewer_done" status
│   ├── reviewed count > 0 (actual work was done, not just setup)
│   ├── review findings files exist on disk
│   ├── {MIN_REVIEW_COVERAGE}% of findings have review scores
│   └── no cluster is stuck in "pending" status
├── on_failure:
│   ├── block merge transition
│   ├── return to review phase
│   └── log which specific checks failed
└── anti-pattern_prevented:
    "creating review_state.json infrastructure and marking
     review phase complete WITHOUT running actual reviewers"
```

### Reviewer Protocol

```
REVIEWER
├── reads: one finding + its original source(s)
├── checks:
│   ├── finding has evidence in source (not hallucinated)
│   ├── confidence score is justified
│   ├── required fields are populated (or have UNK records)
│   └── related artifacts are correctly identified
├── enriches:
│   ├── fills empty optional fields from source data
│   ├── corrects confidence scores based on evidence strength
│   └── adds cross-references discovered during review
├── uses load_full_if hints:
│   └── loads only artifacts/sources relevant to current finding
└── outputs: enriched finding JSON with review metadata
```

### Phase Transition Diagram

```
                    coverage < {COVERAGE_TARGET}%
                    AND iteration <= {MAX_ANALYSIS_RETURNS}
                    ┌────────────────────┐
                    v                    │
[SOURCE] ──> [ANALYSIS] ──> [COVERAGE AUDIT] ──> [REVIEW] ──> [REVIEW GATE] ──> [MERGE] ──> [ARTIFACTS]
                                                      ^              │
                                                      │     gate fails│
                                                      └──────────────┘
```

### Configuration

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `{SOURCE_FORMAT}` | Format of input source materials | markdown, code, JSON | yes |
| `{ARTIFACT_FORMAT}` | Format of output artifacts | YAML, JSON | yes |
| `{COVERAGE_TARGET}` | Minimum % of sources that must produce findings | 80 | yes |
| `{MAX_ANALYSIS_RETURNS}` | Maximum re-analysis iterations for coverage gaps | 2 | yes |
| `{MIN_REVIEW_COVERAGE}` | Minimum % of findings that must be reviewed | 90 | yes |
| `{GATE_SCRIPT}` | Script that enforces review gate | review_gate.py | yes |
| `{FINDINGS_DIR}` | Directory where analysis writes findings | context/findings/ | yes |
| `{REVIEW_STATE_FILE}` | File tracking review progress | review_state.json | yes |
| `{MAX_FINDINGS_PER_REVIEWER}` | Maximum findings per reviewer sub-agent call | 10 | no |

### Decision Rules

| Situation | Action |
|-----------|--------|
| Agent wants to create artifact during analysis | BLOCK -- analysis produces findings only; explain rule |
| Coverage audit shows gaps after max returns | Proceed to review; document gaps as explicit UNK records |
| Review gate fails | Return to review; identify which checks failed; do NOT override |
| Reviewer finds hallucinated finding | Reject finding; create UNK record; do NOT silently fix |
| Reviewer finds missing cross-reference | Enrich the finding with the reference; log the addition |
| Context pressure during review | Use load_full_if hints; load only relevant sources per finding |
| Merge encounters finding without review score | Skip finding; log as unreviewed; do NOT create artifact from it |
| All review scores are 1.0 (perfect) | Suspicious -- verify reviewer is actually checking sources, not rubber-stamping |
| Infrastructure files created but no review work done | Gate catches this -- reviewed_count must be > 0 |

<!-- REFERENCE: load on adoption -->
## Adaptation Guide

### Prerequisites
- A pipeline that extracts structured artifacts from unstructured sources
- Ability to store intermediate findings as JSON files
- A script runner for gate enforcement (bash, python, or equivalent)
- Source materials accessible during review phase (not just during analysis)

### Steps to Adopt
1. Define your findings JSON schema (what analysis produces)
2. Implement the analysis phase: source -> findings JSON, NO artifact creation
3. Implement a simple coverage audit: count sources with findings / total sources
4. Implement the review gate script with at minimum 3 checks: review_state exists, reviewed_count > 0, findings files exist
5. Implement the reviewer: reads finding + source, verifies evidence, enriches gaps
6. Implement the merge phase: reads reviewed findings, creates artifacts
7. Wire the gate: merge script calls gate script first; exits on failure
8. Add bounded re-analysis: track analysis_iteration in state, allow up to `{MAX_ANALYSIS_RETURNS}` returns
9. Monitor review gate failure rate -- if it never fires, verify it is actually running

### What to Customize
- Findings JSON schema (domain-specific fields, confidence scoring)
- Coverage target and thresholds (depends on source density and expected yield)
- Reviewer enrichment logic (what fields can be filled, what cross-references to check)
- Gate checks beyond the minimum (add domain-specific verification)
- Max findings per reviewer call (depends on finding complexity and context budget)

### What NOT to Change
- Three phases are STRICTLY separated -- analysis never creates artifacts
- Artifacts created ONLY in merge phase -- this is the core invariant
- Review gate is mechanical (script), not textual (instruction) -- text rules get skipped under pressure
- Gate checks evidence of actual work, not just infrastructure creation
- Bounded re-analysis -- unbounded retry creates infinite loops
- Findings preserved after merge -- they are the audit trail
- Reviewer accesses original sources -- review without evidence is rubber-stamping

<!-- HISTORY: load for audit -->
## Origin

### spec-creator
- **Findings:** [10] Separation of Analysis, Review, and Merge Phases, [13] Review Gate (Mechanical Enforcement Against Skipping), [46] Coverage Audit with Bounded Re-analysis Loop, [53] Context Compaction Causes Triple Information Loss
- **Discovered through:** A specification extraction pipeline that processes source code into structured artifacts (use cases, business rules, functional requirements). The three-phase separation was established after a critical incident: Claude Code's context compaction compressed a multi-phase pipeline description into "create YAML cards," causing the agent to skip review entirely and write artifacts directly from analysis findings. This caused triple information loss: (1) task creation lost steps, (2) compaction summary simplified domain-specific ordering, (3) agent interpreted simplified text literally. The review gate was added after a second incident where the agent created review_state.json and review_plan.md (infrastructure for review) but marked the review phase complete without running any reviewer sub-agents. The coverage audit with bounded re-analysis emerged from observing that a single analysis pass typically covers 70-80% of sources; 1-2 targeted re-analysis passes bring coverage to 90%+.
- **Evidence:** Without the review gate, the review phase was skipped twice in production runs, causing artifacts with hallucinated content and missing cross-references that took 2-3 days to diagnose and fix. After gate enforcement, zero review skips occurred. Bounded re-analysis (max 2 returns) improved coverage from ~75% to ~92% without creating infinite loops. The phase naming was changed from "artifact_review" to separate "review" + "merge" after the name itself caused confusion (agents interpreted "artifact_review" as "create artifacts").

## Related Patterns
- [Validation Gates](../patterns/validation_gates.md) -- review gate is a specialized validation gate for phase transitions
- [Invariant-Based Testing](../methodologies/invariant_testing.md) -- invariants run after merge as final quality check
- [Maturity Metrics](../methodologies/maturity_metrics.md) -- pipeline phases are where baselines are measured
- [Cognitive Offload](../methodologies/cognitive_offload.md) -- review gate is mechanical work offloaded to a script
- [Closed-Loop Quality System](../methodologies/closed_loop_quality.md) -- quality issues from merge feed back into invariant suite
- [Scenario-First Testing](../methodologies/scenario_first_testing.md) -- end-to-end scenarios test the full three-phase pipeline
- [Monitoring Principles](../best_practices/monitoring_principles.md) -- coverage audit metrics are monitoring signals
- [Adversarial Reflection](../patterns/adversarial_reflection.md) -- adversarial analyst/reviewer roles strengthen the review phase
