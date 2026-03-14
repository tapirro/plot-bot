---
id: cross-loop-protocol
title: Autonomous Loop Protocol
type: pattern
concern: [autonomous-loop]
mechanism: [rotation, state-machine]
scope: per-cycle
lifecycle: [detect, classify, act, reflect, improve]
origin: harvest/multi
origin_findings:
  hilart-ops-bot: [7, 1, 18, 20, 22, 25]
  voic-experiment: [15, 35, 47, 44, 45]
maturity: draft
domain: cross-agent/patterns
confidence: observed
basis: "cross-agent pattern from hilart-ops-bot and voic-experiment harvests"
---

# Autonomous Loop Protocol

<!-- CORE: load always -->
## Problem

Autonomous agents must cycle through monitoring, self-review, investigation, and improvement, but without an explicit protocol they default to one of two failure modes: narrow focus (optimizing one domain while others degrade) or thrashing (jumping between concerns without completing any).

At scale -- 100+ loops over weeks of operation -- subtler problems emerge. Quality drifts as small regressions accumulate unnoticed across loops. Cumulative impact goes unmeasured because individual loops are evaluated in isolation, never as a series. Regression oscillation appears in feedback loops: fix A breaks B, fix B breaks A, and the agent cycles indefinitely without converging.

Improvement work is perpetually deferred. Without a dedicated slot in the schedule, self-improvement items sit in a backlog that grows monotonically. The agent never pauses execution to sharpen its own tools, recalibrate its thresholds, or reflect on whether its approach is working. Over time, this compounds: the agent operates with increasingly stale methods on increasingly complex problems.

## Solution

The protocol provides two merged approaches optimized for different agent types, unified by shared principles: structured rotation, mandatory periodic reflection, quantitative self-metrics, and persistent state.

**Rotation-based scheduling** (for monitoring agents) defines a fixed-position rotation across monitored domains, with dedicated META, INVESTIGATE, and IMPROVE positions built into the schedule. A state machine enforces that every domain receives fair coverage before meta-analysis occurs, and that improvement work is a first-class scheduled activity rather than an afterthought. The DECAY+RECHECK innovation between assessment and detection prevents false positives from stale confidence scores.

**Cycle-based scheduling** (for development agents) groups loops into fixed-length cycles where the first loop is always META -- a mandatory retrospective that scores prior impact, runs the quality system, performs research, and plans the next cycle. Regular loops follow a hypothesis-to-commit pipeline where each loop produces exactly one commit, preserving bisectability. BOLD loops provide isolated environments for risky experiments that may fail without contaminating the main line.

Both modes track agent self-metrics (coverage, detection rate, signal-to-noise for monitoring; impact scores, commit rate, test pass rate for development). A zero-CRAFT trigger automatically shifts focus to stabilization when average impact drops below a threshold, preventing quality death spirals. North star metrics drive prioritization across all loop types, ensuring that every loop's work is evaluated against the agent's ultimate success criteria.

## Implementation

### Structure

```
LOOP_STATE
├── loop_count: int
├── cycle_count: int
├── current_position: int (0..N-1)      # rotation mode
├── cycle_position: int (0..CYCLE_LEN)  # cycle mode
├── phase: string (current state machine phase)
├── incidents: list
├── bot_metrics: object
├── impact_scores: list[float]           # per-loop impact (1-5)
├── investigation_backlog: list
└── improvement_backlog: list
```

### Mode A: Rotation Schedule (Monitoring Agents)

```
Position 0: {UNIT_1}          <- domain monitoring
Position 1: {UNIT_2}          <- domain monitoring
Position 2: {UNIT_3}          <- domain monitoring
...
Position N: META              <- aggregate + cross-correlate + self-review
Position N+1: INVESTIGATE     <- select from backlog + execute
Position N+2: IMPROVE         <- select from backlog + execute
```

Every `{META_FREQUENCY}`th META = Retrospective (deeper self-analysis).

### Mode B: Cycle Schedule (Development Agents)

```
Cycle of {CYCLE_LENGTH} loops:
  Position 0: META            <- retrospective + planning
  Position 1: {LOOP_TYPE}     <- regular execution
  Position 2: {LOOP_TYPE}     <- regular execution
  ...
  Position N: {LOOP_TYPE}     <- regular execution
```

META is always the first loop of each cycle. The remaining loops execute against the plan established in META.

#### META Loop Protocol

META loops perform five operations in sequence:

| Step | Action | Detail |
|------|--------|--------|
| 1. Retrospective Scoring | Score each loop in prior cycle | Impact 1-5 scale per loop |
| 2. Quality System Full Run | Execute `{QUALITY_SYSTEM_COMMAND}` in `--full` mode | Comprehensive quality analysis |
| 3. Research | Perform `{RESEARCH_COUNT}` web searches on {NORTH_STAR_TOPIC} | Gather external context |
| 4. Cycle Planning | Write `{NEXT_CYCLE_PLAN}` | Prioritized plan for next cycle |
| 5. Digest | Generate cycle summary + review | Cumulative progress report |

**Impact threshold**: If average impact score for completed cycle < `{MIN_IMPACT_THRESHOLD}`, next cycle enters zero-CRAFT mode (no creative work, only stabilization and fixes).

### Unit Loop State Machine (Monitoring)

```
IDLE -> ASSESS -> DECAY -> RECHECK -> DETECT -> [ANALYZE] -> REPORT -> [ESCALATE] -> RECORD -> IDLE
```

| Phase | Action |
|-------|--------|
| IDLE | Waiting for rotation |
| ASSESS | Fetch fresh data from all sources |
| DECAY | Apply confidence decay to existing incidents |
| RECHECK | Re-evaluate existing incidents with fresh data |
| DETECT | Compare metrics against baselines/thresholds |
| ANALYZE | (conditional) Drill into detected anomalies |
| REPORT | Generate loop report artifact |
| ESCALATE | (conditional) Send escalation if gates pass |
| RECORD | Update state.json, save snapshot |

Key innovation: **DECAY + RECHECK** between ASSESS and DETECT prevents false positives from stale confidence.

### Regular Loop Pipeline (Development)

```
HYPOTHESIS -> PLAN -> IMPLEMENT -> TEST -> VERIFY -> REPORT -> COMMIT -> QUALITY_GATE
```

| Phase | Action |
|-------|--------|
| HYPOTHESIS | Define what this loop will achieve and why |
| PLAN | Break into steps, estimate scope |
| IMPLEMENT | Execute the plan |
| TEST | Run test suite |
| VERIFY | Confirm hypothesis was validated |
| REPORT | Document what was done and learned |
| COMMIT | One loop = one commit, never batched |
| QUALITY_GATE | Run `{QUALITY_SYSTEM_COMMAND}` in `--gate` mode (advisory) |

**One loop = one commit.** Never batch multiple loops into a single commit. This preserves bisectability and traceability.

### BOLD Loops

BOLD loops allow exploratory work that may fail without contaminating the main line:

| Property | Value |
|----------|-------|
| Isolation | `{ISOLATION_MECHANISM}` (e.g., git worktree) |
| Permission | Failure is acceptable |
| Merge condition | Only if tests pass and impact > `{BOLD_MERGE_THRESHOLD}` |
| Cleanup | Discard isolation environment on failure |

Use BOLD loops for: risky refactors, architectural experiments, speculative features.

### Meta-Loop Phases (Monitoring)

```
AGGREGATE -> CROSS-CORRELATE -> SELF-REVIEW -> TRIAGE -> RECORD
```

| Phase | Action |
|-------|--------|
| AGGREGATE | Summarize last N unit reports |
| CROSS-CORRELATE | Check inter-unit dependencies and cascading effects |
| SELF-REVIEW | Update agent self-metrics, propose taxonomy evolution |
| TRIAGE | Promote incidents to investigation backlog |
| RECORD | Update state |

### Improve Loop

```
IDLE -> ASSESS -> SELECT -> EXECUTE -> RECORD -> IDLE
```

Improvement categories by priority: `TOOL > THRESHOLD > KNOWLEDGE > METHOD`

- TOOL: Fix/create a script or query (auto-approvable)
- THRESHOLD: Recalibrate a threshold based on data (auto-approvable)
- KNOWLEDGE: Document a finding (auto-approvable)
- METHOD: Change a methodology (requires owner approval -> BLOCKED until approved)

Auto-stale: P0 after `{STALE_P0}` loops, P1 after `{STALE_P1}`, P2 after `{STALE_P2}`.

### North Star Metrics

Each agent defines `{NORTH_STAR_METRICS}` that represent its ultimate success criteria. These metrics:

- Drive prioritization in META loop planning
- Weight incident severity through `{NORTH_STAR_MULTIPLIER}` (see Incident Taxonomy)
- Determine BOLD loop merge thresholds
- Gate cycle-level success/failure

Example configuration:
```
{NORTH_STAR_METRICS}:
  - name: "{METRIC_1_NAME}"
    target: {METRIC_1_TARGET}
    weight: {METRIC_1_WEIGHT}
  - name: "{METRIC_2_NAME}"
    target: {METRIC_2_TARGET}
    weight: {METRIC_2_WEIGHT}
```

### Agent Self-Metrics

5 metrics tracked per loop:

| Metric | Description | Target |
|--------|------------|--------|
| Coverage | % of domains monitored this cycle | > `{COVERAGE_TARGET}`% |
| Detection Rate | Incidents found (total and confirmed) | Growing |
| Avg Confidence | Mean confidence of active incidents | > `{CONFIDENCE_TARGET}` |
| Latency | Time from data arrival to report | < `{LATENCY_TARGET}` |
| Signal-to-Noise | Confirmed incidents / total incidents | > `{SN_TARGET}`:1 |

For development agents, additional metrics:

| Metric | Description | Target |
|--------|------------|--------|
| Avg Impact | Mean impact score across cycle | > `{MIN_IMPACT_THRESHOLD}` |
| Commit Rate | Loops resulting in successful commits | > `{COMMIT_RATE_TARGET}`% |
| Test Pass Rate | % of loops with green test suite | > `{TEST_PASS_TARGET}`% |

### Parallel Subagent Strategy

For evaluating independence of components or verifying isolation:

```
1. Spawn {PARALLEL_COUNT} subagents
2. Each operates on an isolated copy ({ISOLATION_MECHANISM})
3. Each runs the same loop pipeline independently
4. Compare results to detect coupling or dependency leaks
5. Merge only results that converge
```

Use when: testing whether a refactor is truly isolated, evaluating multiple approaches simultaneously.

### State Persistence

```
{STATE_DIR}/
├── state.json              <- master state (loop_count, cycle_count, phase, backlog counters)
├── snapshots/{unit}.json   <- last data snapshot per unit
├── cycle_plans/
│   └── cycle_{N}.md        <- plan for each cycle
├── impact_scores.jsonl     <- impact score per loop (append-only)
├── reports/
│   ├── incidents/active.jsonl  <- append-only incident log
│   └── bot_metrics.jsonl       <- self-metrics per loop
└── digests/
    └── cycle_{N}_digest.md <- cycle summary
```

Current state = JSON (single file, overwritten). History = JSONL (append-only, one line per event).

### Composable Skills

Skills are stateless routines organized by loop type:

| Loop Type | Skills |
|-----------|--------|
| Unit loop | assess, detect, analyze, report |
| Meta-loop | aggregate, cross_correlate, self_review, triage |
| META (dev) | retrospective, quality_full, research, plan, digest |
| Regular (dev) | hypothesis, implement, test, verify, commit, quality_gate |
| Investigation | assess, select, execute |
| Improve | execute |
| Utility | escalate, daily_digest |

Design: JSON/YAML in -> artifacts out. Output of one skill feeds input of next.

### Configuration

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `{UNIT_N}` | Names of monitored domains | "Partnerships", "Logistics" | mode A |
| `{META_FREQUENCY}` | Retrospective every Nth META | 3 | mode A |
| `{CYCLE_LENGTH}` | Loops per cycle (including META) | 5 | mode B |
| `{LOOP_TYPE}` | Default regular loop type | "regular", "bold" | mode B |
| `{QUALITY_SYSTEM_COMMAND}` | Command to run quality checks | "./quality.sh" | mode B |
| `{RESEARCH_COUNT}` | Web searches per META | 3 | mode B |
| `{NORTH_STAR_TOPIC}` | Research topic for META | Domain-specific | mode B |
| `{MIN_IMPACT_THRESHOLD}` | Avg impact below this = zero-CRAFT | 3.0 | mode B |
| `{BOLD_MERGE_THRESHOLD}` | Min impact to merge BOLD | 3 | mode B |
| `{ISOLATION_MECHANISM}` | How BOLD loops isolate | "git worktree" | mode B |
| `{PARALLEL_COUNT}` | Subagents for parallel eval | 2-4 | no |
| `{STALE_P0}` | Auto-stale P0 improvements after N loops | 10 | yes |
| `{STALE_P1}` | Auto-stale P1 after N loops | 20 | yes |
| `{STALE_P2}` | Auto-stale P2 after N loops | 30 | yes |
| `{COVERAGE_TARGET}` | Min coverage % | 80 | mode A |
| `{CONFIDENCE_TARGET}` | Min average confidence | 0.8 | mode A |
| `{LATENCY_TARGET}` | Max detection latency | 30min | mode A |
| `{SN_TARGET}` | Min signal-to-noise ratio | 3 | mode A |
| `{COMMIT_RATE_TARGET}` | Min commit success % | 80 | mode B |
| `{TEST_PASS_TARGET}` | Min test pass % | 90 | mode B |
| `{STATE_DIR}` | Directory for state persistence | loops/current/ | yes |

### Decision Rules

- **Mode selection**: Use Mode A (rotation) for monitoring/ops agents. Use Mode B (cycles) for development/build agents.
- **BOLD vs regular**: Default to regular. Use BOLD only when the loop involves changes with > 30% chance of failure.
- **Zero-CRAFT trigger**: If avg impact for a completed cycle < `{MIN_IMPACT_THRESHOLD}`, the next cycle must focus exclusively on stabilization (bug fixes, test coverage, debt reduction).
- **Cycle length**: Start with `{CYCLE_LENGTH}` = 5. Increase to 7-10 as the agent matures and META overhead amortizes better.
- **Parallel subagents**: Only use when verifying isolation. High resource cost.

<!-- REFERENCE: load on adoption -->
## Adaptation Guide

### Prerequisites
- Clear definition of monitored domains (units) OR development objectives (north star)
- Data sources accessible for each unit OR a codebase with test suite
- Incident entity model (see Confidence & Severity Model) OR quality system (see Closed-Loop Quality System)
- State persistence mechanism (file system or database)
- For Mode B: version control system supporting isolation (worktrees or branches)

### Steps to Adopt

**For Mode A (Monitoring):**
1. Define units (domains to monitor) -- typically 2-5
2. Build rotation schedule: N units + META + INVESTIGATE + IMPROVE
3. Implement unit loop state machine (start simple: ASSESS -> DETECT -> REPORT -> RECORD)
4. Add DECAY + RECHECK phases (critical for reducing FP)
5. Implement meta-loop (start with AGGREGATE + SELF-REVIEW)
6. Add investigation loop with backlog and session budget
7. Add improvement loop with priority categories
8. Implement state persistence (state.json + JSONL history)
9. Define and track 5 self-metrics

**For Mode B (Development):**
1. Define `{NORTH_STAR_METRICS}` -- what matters most
2. Set `{CYCLE_LENGTH}` (start with 5)
3. Implement regular loop pipeline: hypothesis -> plan -> implement -> test -> verify -> commit
4. Implement META loop: retrospective scoring -> quality system -> research -> plan -> digest
5. Add quality gate (post-commit advisory check)
6. Track impact scores per loop (1-5 scale)
7. Implement zero-CRAFT trigger for low-impact cycles
8. Add BOLD loop support with `{ISOLATION_MECHANISM}`
9. Define and track development self-metrics

### What to Customize
- Number and names of units (Mode A)
- Data sources and queries per unit (Mode A)
- North star metrics and weights (Mode B)
- Cycle length and META frequency
- Specific skills per loop type
- Self-metric targets
- Stale timeout per priority
- BOLD loop merge threshold

### What NOT to Change
- The rotation principle: fair coverage of ALL units before META (Mode A)
- DECAY + RECHECK in unit loop -- removing these guarantees more FP (Mode A)
- One loop = one commit -- batching destroys traceability (Mode B)
- META as mandatory periodic reflection -- not optional (both modes)
- Impact scoring and zero-CRAFT trigger -- prevents quality drift (Mode B)
- Self-metrics as a concept -- agents must measure themselves (both modes)
- IMPROVE as a dedicated loop position -- not an afterthought (Mode A)
- State = JSON (current) + JSONL (history) separation (both modes)

<!-- HISTORY: load for audit -->
## Origin

### hilart-ops-bot
- **Findings:** [7] Loop Protocol, [1] Entity System, [18] IMPROVE Loop, [20] Composable Skills, [22] State & Snapshot System, [25] Bot Self-Metrics
- **Discovered through:** 9 loops of iterative development. The 6-position rotation emerged from needing fair unit coverage + time for meta-analysis + investigation + improvement. The DECAY+RECHECK innovation was added after false positives from stale confidence in early loops.
- **Evidence:** Coverage maintained at 100% (target >80%). Self-improvement backlog grew to 7 items from 3 investigations, demonstrating the system generates improvements. S/N and confidence identified as "hard problems" through self-metrics tracking.

### voic-experiment
- **Findings:** [15] LOOPMODE v2, [35] META Loop Protocol, [47] BOLD Loops, [44] North Star Metrics, [45] Parallel Subagent Strategy
- **Discovered through:** 226 loops across 45 cycles with average impact 3.1. The 5-loop cycle structure (META + 4 regular) emerged as the optimal balance between reflection and execution. BOLD loops added after observing that some valuable experiments were avoided due to risk of breaking the main line. Zero-CRAFT trigger added after cycles with avg impact < 3.0 showed accumulating technical debt.
- **Evidence:** 226 loops completed with sustained quality. Impact scoring enabled data-driven cycle planning. BOLD loops unlocked risky-but-valuable experiments. Zero-CRAFT prevented quality death spirals.

## Related Patterns
- [Confidence & Severity Model](../methodologies/confidence_severity.md) -- confidence lifecycle operates within loop phases
- [Metric Classification](../methodologies/metric_classification.md) -- determines comparison method in DETECT phase
- [Validation Gates](../patterns/validation_gates.md) -- gates check at phase transitions
- [Incident Taxonomy](../patterns/incident_taxonomy.md) -- classification used in DETECT
- [Closed-Loop Quality System](../methodologies/closed_loop_quality.md) -- quality system invoked by META and gate loops
- [Three-Tier Backlog Management](../methodologies/backlog_management.md) -- cycle selection drives what each loop works on
- [Hypothesis-Driven Experimentation](../methodologies/experimentation.md) -- each regular loop follows hypothesis-verify rhythm
- [Human Review Digest](../patterns/human_review_digest.md) -- digest generated at REPORT phase
- [Observability Engine](../patterns/observability_engine.md) -- spans instrument loop phases
- [Provider Resilience](../patterns/provider_resilience.md) -- provider health feeds into self-metrics
- [Artifact-Centric Interface](../patterns/artifact_centric_interface.md) -- loop state persistence follows Layer 1
- [Monitoring Principles](../best_practices/monitoring_principles.md) -- principles for self-measurement
- [Append-Only Audit Trail](../patterns/append_only_audit.md) -- bounded retry loops operate safely with idempotent append/upsert
- [Cognitive Offload](../methodologies/cognitive_offload.md) -- loop efficiency depends on mechanical work being scripted
- [Repository-as-Product (RaP)](../methodologies/rap_methodology.md) -- bounded loops are scripted from text protocols via RaP cycle
