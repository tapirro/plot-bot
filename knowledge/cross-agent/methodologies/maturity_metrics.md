---
id: cross-maturity-metrics
title: Maturity Metrics
type: methodology
concern: [observability]
mechanism: [scoring-model]
scope: system
lifecycle: [reflect, improve]
origin: harvest/spec-creator
origin_findings: [18, 70, 80]
maturity: draft
domain: cross-agent/methodologies
confidence: observed
basis: "distilled from spec-creator harvest, 104 findings across 16+ agent sessions"
---

# Maturity Metrics

<!-- CORE: load always -->
## Problem

Agent systems lack objective measures of development maturity. When asked "should we invest in more tooling, better testing, or stabilization?" teams have no quantitative basis for the decision. They guess, or they invest in whichever problem is most visible or most recently painful.

Without measurable signals, common failure modes go undetected. A system may have extensive tooling but no tests to verify correctness -- stable-looking output that is actually untested. Or it may have thorough tests but poor automation, with the agent manually performing operations that scripts should handle. These imbalances are invisible without orthogonal metrics that measure each dimension independently.

Effort allocation becomes reactive rather than strategic. The loudest problem gets attention regardless of its actual leverage. A team may spend weeks optimizing a pipeline stage that contributes 5% of total time while ignoring an unmeasured overhead that contributes 70%. Without baselines and metrics, there is no way to know which investment will produce the highest return.

## Solution

Five normalized (0..1) orthogonal metrics form a maturity profile that covers the essential dimensions of agent system development. Kappa (Context Compression) measures how much domain knowledge is externalized into deterministic code versus held in agent instructions. Delta (Process Determinism) measures what fraction of mechanical actions are automated via scripts. Iota (Invariant Density) measures how well "correct output" is formally defined through testable properties. Phi (Semantic Focus) measures what fraction of agent effort produces domain value versus mechanical overhead. Sigma (Boundary Convergence) measures output stability across runs.

Two derived signals guide investment: Readiness (R = minimum of kappa, delta, iota) identifies the bottleneck dimension that limits overall system maturity, and Imbalance (U = max minus min across all five) detects lopsided development where one dimension is far ahead of others. Diagnostic combinations of metric values reveal specific system states -- for example, high sigma with low iota signals "false confidence" where output looks stable but correctness is undefined.

Performance baselines (tokens per batch, tool calls per phase, time per operation) provide the empirical data that feeds these metrics. Baselines are established through measured runs before any optimization, enabling data-driven decisions about where to invest effort. The profile is recomputed at each release, creating a trajectory that shows whether the system is maturing in a balanced way or developing blind spots.

## Implementation

### Structure

```
MATURITY_PROFILE
├── kappa (Context Compression)
│   ├── formula: code_knowledge / total_knowledge
│   ├── range: 0..1
│   ├── interpretation: 0 = all knowledge in agent's head (instructions),
│   │                   1 = all knowledge encoded in code (scripts, schemas, validators)
│   └── measures: how much domain knowledge is externalized into deterministic artifacts
├── delta (Process Determinism)
│   ├── formula: scripted_actions / total_actions
│   ├── range: 0..1
│   ├── interpretation: 0 = all actions performed manually by agent,
│   │                   1 = all mechanical actions automated via scripts
│   └── measures: what fraction of the process is deterministic
├── iota (Invariant Density)
│   ├── formula: defined_invariants / I_exp
│   ├── range: 0..1
│   ├── I_exp = ({ARTIFACT_TYPES} × 3) + ({MODEL_RELATIONS} × 2) + 5
│   ├── interpretation: 0 = "correct" is undefined,
│   │                   1 = full property coverage
│   └── measures: how well "correct output" is formally defined
├── phi (Semantic Focus)
│   ├── formula: semantic_tokens / total_tokens
│   ├── range: 0..1
│   ├── interpretation: 0 = all effort spent on mechanics (file ops, formatting),
│   │                   1 = all effort on meaningful work (extraction, judgment)
│   └── measures: what fraction of agent effort produces domain value
└── sigma (Boundary Convergence)
    ├── formula: 1 - (delta_artifacts_between_runs / total_artifacts)
    ├── range: 0..1
    ├── interpretation: 0 = every run produces different output,
    │                   1 = output fully converged across runs
    └── measures: stability of the product itself
```

```
DERIVED_SIGNALS
├── R (Readiness) = min(kappa, delta, iota)
│   └── bottleneck: whichever metric is lowest blocks overall readiness
├── U (Imbalance) = max(all five) - min(all five)
│   └── target: U < 0.3 (balanced investment across dimensions)
└── diagnostic_combinations: see table below
```

### Diagnostic Combinations

| Combination | Signal | Interpretation | Recommended Action |
|-------------|--------|----------------|-------------------|
| high kappa + low delta | "Coded but not automated" | Knowledge is in scripts/schemas, but agent still runs things manually | Wire scripts into the pipeline; agent should call tools, not replicate them |
| high kappa + low phi | "Toolkit unused" | Tools exist but agent burns tokens on mechanics anyway | Review instructions; add explicit "use script X" directives; check if agent knows toolkit |
| high sigma + low iota | "False confidence" | Output looks stable but correctness is undefined | Add invariants urgently; stability without tests is coincidence |
| low kappa + high delta | "Cargo-cult automation" | Scripts exist but encode wrong/outdated knowledge | Review script logic against current domain understanding |
| high iota + low sigma | "Correct but unstable" | Tests exist and output varies between runs | Investigate nondeterminism sources; tighten prompts or add convergence constraints |
| low U (< 0.2) | "Balanced" | All dimensions developing evenly | Continue current trajectory |
| high U (> 0.5) | "Lopsided" | One dimension far ahead of others | Invest in the lagging dimension |

### Performance Baselines

Before optimizing, establish quantitative baselines per phase:

```
BASELINE_RECORD
├── phase: string                    # pipeline phase name
├── metric: string                   # what is measured
├── value: float                     # measured value
├── unit: string                     # tokens | seconds | tool_calls | items
├── measured_at: datetime
├── conditions: string               # model, concurrency, data size
└── notes: string                    # context for interpretation
```

| Metric | Unit | Purpose |
|--------|------|---------|
| Tokens per batch | tokens | Context efficiency; tracks phi over time |
| Tool calls per phase | count | Process determinism; tracks delta |
| Time per operation | seconds | Bottleneck identification |
| Instructions token count | tokens | Context compression; tracks kappa |
| Invariant count | count | Testing coverage; tracks iota |
| Artifact delta between runs | count | Stability; tracks sigma |

Baselines enable data-driven optimization: instead of guessing where effort should go, measure each metric, compute the profile, and invest in the lowest dimension.

### Configuration

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `{ARTIFACT_TYPES}` | Number of distinct artifact types in the system | 7 | yes |
| `{MODEL_RELATIONS}` | Number of defined relationships between artifact types | 12 | yes |
| `{MEASUREMENT_FREQUENCY}` | How often to recompute the profile | per-release | yes |
| `{IMBALANCE_THRESHOLD}` | U value above which investment rebalancing is recommended | 0.3 | yes |
| `{READINESS_TARGET}` | Minimum R value for production readiness | 0.6 | no |
| `{BASELINE_SAMPLE_SIZE}` | Number of runs to average for stable baselines | 3 | yes |

### Decision Rules

| Situation | Action |
|-----------|--------|
| R < 0.3 | System not ready for production use; invest in the min(kappa, delta, iota) dimension |
| R >= 0.6 and U < 0.3 | System is balanced and mature; focus on sigma (convergence) |
| U > `{IMBALANCE_THRESHOLD}` | Identify lagging metric; prioritize it in next development cycle |
| kappa rising, phi flat | Scripts being created but not adopted; update agent instructions to use them |
| iota < 0.5 after 3+ runs | Testing investment critically low; add invariants before adding features |
| sigma < 0.5 after baselines established | Nondeterminism too high; investigate LLM temperature, prompt variation, input drift |
| No baselines exist | Cannot compute phi or delta reliably; establish baselines first (1-2 measured runs) |

<!-- REFERENCE: load on adoption -->
## Adaptation Guide

### Prerequisites
- At least 2-3 completed pipeline runs with logs (for baseline measurement)
- Defined artifact types and their relationships (for I_exp calculation)
- Access to run logs with token counts and tool call records (for phi, delta)
- A way to diff outputs between runs (for sigma)

### Steps to Adopt
1. Enumerate artifact types and relations in your system to compute I_exp
2. Run 2-3 pipeline executions with full logging enabled
3. Compute initial baselines: tokens per batch, tool calls per phase, time per operation
4. Count existing invariants (schema checks, coverage thresholds, stability checks)
5. Compute all five metrics from the logged data
6. Identify the bottleneck (lowest metric) and the imbalance (U)
7. Set improvement targets for the next 2-3 development cycles
8. Recompute after each release to track trajectory

### What to Customize
- `{ARTIFACT_TYPES}` and `{MODEL_RELATIONS}` (domain-specific counts)
- Measurement frequency (per-run for active development, per-release for stable systems)
- Imbalance threshold (tighter for critical systems, looser for experimental ones)
- Additional diagnostic combinations for your specific failure modes
- Baseline metrics beyond the standard set (e.g., cost per run, error rate)

### What NOT to Change
- Five metrics are orthogonal -- do not merge or substitute them
- All metrics normalized to 0..1 -- do not use raw counts
- R = min(), not average -- the weakest dimension is the bottleneck
- U = max - min -- measures spread, not absolute level
- Baselines before optimization -- never optimize without measurement
- Every metric must be computable from observable data (logs, files, diffs) -- no subjective scoring

<!-- HISTORY: load for audit -->
## Origin

### spec-creator
- **Findings:** [18] Maturity Metrics System (5 orthogonal metrics), [70] Performance Baselines Enable Data-Driven Optimization, [80] Cognitive Offload Architecture
- **Discovered through:** Building an extraction pipeline over 16+ iterations. Early development invested heavily in tooling (high kappa) while ignoring invariants (low iota), creating false confidence in stable-looking but untested outputs. The five-metric model was formulated during a retrospective that asked "how do we know where to invest next?" Performance baselines (67.2 sec/card wall-clock) enabled targeted optimization that reduced processing time by ~50% for 74% of cards. Cognitive offload analysis revealed ~60% of pre-extraction effort was mechanical, guiding script creation priorities that shifted delta from ~0.3 to ~0.7.
- **Evidence:** Cognitive offload analysis reduced instructions from ~25K to ~8K tokens (kappa improvement). Tool calls per batch dropped from 15-20 to 4-6 (delta improvement). Baselines revealed near-sequential parallelism (coefficient 0.93) despite max_parallel=2, identifying the real bottleneck. Without these metrics, the team would have optimized the wrong thing.

## Related Patterns
- [Invariant-Based Testing](../methodologies/invariant_testing.md) -- iota metric measures invariant coverage; testing methodology provides the invariants
- [Cognitive Offload](../methodologies/cognitive_offload.md) -- kappa and delta metrics quantify what cognitive offload achieves
- [Analysis-Review-Merge Pipeline](../methodologies/analysis_review_merge.md) -- pipeline phases are where baselines are measured
- [Monitoring Principles](../best_practices/monitoring_principles.md) -- maturity metrics are a form of system self-monitoring
- [Closed-Loop Quality System](../methodologies/closed_loop_quality.md) -- quality loop generates the run data that feeds sigma
- [Validation Gates](../patterns/validation_gates.md) -- gate pass rates contribute to iota measurement
- [Scenario-First Testing](../methodologies/scenario_first_testing.md) -- scenarios are a source of defined invariants
- [Adversarial Reflection](../patterns/adversarial_reflection.md) -- adversarial review quality feeds into sigma (boundary convergence)
- [Knowledge Layer Architecture](../methodologies/knowledge_layers.md) -- knowledge layer maturity directly maps to kappa metric
