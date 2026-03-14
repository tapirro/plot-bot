---
id: cross-phase-gate
title: Phase Gate & Task DAG
type: pattern
concern: [pipeline-control, validation-gating]
mechanism: [gate-pyramid, pipeline]
scope: per-cycle
lifecycle: [decide, act]
origin: harvest/spec-creator
origin_findings: [5, 44, 45, 46, 50, 65, 66, 73]
maturity: draft
domain: cross-agent/patterns
confidence: observed
basis: "distilled from spec-creator harvest, 104 findings across 16+ agent sessions"
---

# Phase Gate & Task DAG

<!-- CORE: load always -->
## Problem

Text-based phase transition rules are insufficient for controlling LLM agent pipelines. Agents skip phases, confuse preparation with execution, and mark phases complete based on infrastructure creation rather than actual work.

This is not a theoretical risk. In documented incidents, an agent created review infrastructure files (review_state.json, review_plan.md) and marked the review phase complete without running any reviewer subagents. This happened twice despite explicit textual rules forbidding it. The agent interpreted "set up the review phase" as completing the review phase -- a distinction that text rules cannot enforce mechanically.

A compounding factor is task pollution: when 30+ subtasks are created alongside 7 phase-level tasks, the phase tasks become invisible. The agent loses track of which tasks represent pipeline phases and which are subtasks within a phase, leading to premature phase transitions and skipped verification steps.

## Solution

Replace text-based phase rules with **mechanical enforcement** through three interlocking mechanisms:

1. **Task DAG with `blockedBy` dependencies** -- one Task per pipeline phase, created at pipeline start with sequential dependencies. Phase N+1 physically cannot begin until Phase N is marked complete. This makes skipping a phase impossible at the system level, not just discouraged at the instruction level.

2. **Hierarchical task naming** (`[Phase N] Title` for phases, `[N.M]` for subtasks) -- ensures phase-level tasks remain visually distinct from the subtasks they contain. The orchestrator can always identify which tasks represent pipeline phases.

3. **Gate scripts** that verify real work products, not infrastructure files. A gate script checks conditions like `totals.reviewed > 0` -- creating a review_state.json does not satisfy this gate. Gates are added empirically, only at transitions where skipping has actually occurred.

The pattern costs approximately 2 minutes of setup time and prevents multi-hour pipeline violations. Forward-only movement is enforced with bounded exceptions (e.g., coverage audit can return to analysis, but only N times).

## Implementation

### Structure

```
PIPELINE_DAG
├── phases: list[PHASE_DEFINITION]
│   ├── id: int
│   ├── name: string
│   ├── gate_script: path | null
│   └── max_returns: int             # 0 = no returns allowed
├── task_prefix: "[Phase {N}]"
├── subtask_prefix: "[{N}.{M}]"
└── enforcement: "blockedBy" task dependencies

PHASE_DEFINITION
├── id: int                           # sequential phase number
├── name: string                      # descriptive name (verbs, not nouns)
├── gate_script: path | null          # script that checks completion
├── required_artifacts: list[path]    # files that must exist
├── required_conditions: list[check]  # scripts/checks that must pass
├── max_returns: int                  # bounded retry from later phases
└── on_failure: "block transition"

GATE_CHECK
├── phase_id: int
├── required_artifacts: list[path]    # files that must exist
├── required_conditions: list[check]  # scripts that must pass
└── on_failure: "block transition"
```

### Task DAG Creation

At pipeline start, the orchestrator creates **one Task per phase** with sequential `blockedBy` dependencies:

```
[Phase 1] {PHASE_1_NAME}                          ← no dependencies
[Phase 2] {PHASE_2_NAME}          blockedBy: [1]
[Phase 3] {PHASE_3_NAME}          blockedBy: [2]
[Phase 4] {PHASE_4_NAME}          blockedBy: [3]
[Phase 5] {PHASE_5_NAME}          blockedBy: [4]
...
[Phase N] {PHASE_N_NAME}          blockedBy: [N-1]
```

This makes it **physically impossible** to start Phase N+1 before Phase N completes. Text rules are suggestions; `blockedBy` is enforcement.

### Hierarchical Task Naming

| Level | Format | Example | Purpose |
|-------|--------|---------|---------|
| Phase | `[Phase N] Title` | `[Phase 4] Review findings` | Top-level pipeline task |
| Subtask | `[N.M] Subtitle` | `[4.1] Review cluster C1` | Work item within phase |
| Sub-subtask | `[N.M.K] Detail` | `[4.1.2] Review part 2` | Granular step |

**Why this matters:** Without prefixes, 30+ subtasks bury the 7 phase-level tasks. The orchestrator loses orientation and confuses "preparing for phase" with "executing phase."

```
WITHOUT NAMING          WITH NAMING
┌─────────────────┐     ┌──────────────────────────┐
│ Review findings  │     │ [Phase 4] Review findings │  ← PHASE
│ Create state     │     │ [4.1] Review cluster C1   │  ← subtask
│ Plan waves       │     │ [4.2] Review cluster C2   │  ← subtask
│ Run reviewer C1  │     │ [Phase 5] Merge artifacts │  ← PHASE
│ Run reviewer C2  │     │ [5.1] Merge cluster C1    │  ← subtask
│ Merge artifacts  │     │ [5.2] Merge cluster C2    │  ← subtask
│ Merge C1         │     └──────────────────────────┘
│ Merge C2         │      Phases visually distinct
└─────────────────┘
 Which are phases?
```

### Infrastructure Creation != Phase Completion

Creating configuration files for a phase is NOT completing the phase. Gate scripts verify that **real work was performed**:

| What agents do | What gates check |
|----------------|-----------------|
| Create `review_state.json` | `totals.reviewed > 0` |
| Create `review_plan.md` | All clusters have `reviewer_done` status |
| Set up `{OUTPUT_DIR}` | Findings files actually exist on disk |
| Mark task as "done" | Gate script returns exit code 0 |

### Gate Script Pattern

```
gate_check({PHASE_NAME}):
    for artifact in required_artifacts:
        if not exists(artifact):
            FAIL("Missing: {artifact}")

    for condition in required_conditions:
        result = run(condition)
        if result.exit_code != 0:
            FAIL("Condition failed: {condition.description}")

    PASS("Phase {PHASE_NAME} complete")
```

Gate scripts are added **only where empirical data shows failures**, not preemptively at every boundary. If a phase has never been skipped, a gate script adds unnecessary complexity.

### Forward-Only Pipeline

The pipeline moves **strictly forward**. Return to an earlier phase requires:
1. Explicit user command, OR
2. A defined bounded feedback loop (e.g., coverage audit can return to analysis)

```
Phase 1 → Phase 2 → Phase 3 → Phase 4 → Phase 5 → Phase 6 → Phase 7
                                  ↑                    │
                                  └────────────────────┘
                                  Coverage audit return
                                  (max {MAX_RETURNS} times)
```

The `analysis_iteration` counter in session_state tracks returns. When it reaches `{MAX_RETURNS}`, the pipeline proceeds forward regardless of coverage gaps.

### DAG Recreation on Resume

Task IDs are **unstable between sessions** (task systems assign new IDs). On resume:
1. Read `session_state.json` to determine current phase
2. Recreate the Task DAG with new IDs
3. Mark completed phases as done
4. Resume from the current phase

### Decision Rules

| Situation | Action |
|-----------|--------|
| Pipeline starts | Create N Tasks, each `blockedBy` previous |
| Before marking phase complete | Run `gate_script` if defined |
| Gate fails | Phase stays `in_progress`; fix issues; re-run gate |
| Subtasks created | Use `[N.M]` prefix -- never bare titles |
| Infrastructure files created | NOT phase completion; gate must verify real output |
| Agent wants to skip ahead | Blocked by Task DAG -- physically impossible |
| Phase never been skipped | No gate script needed -- add only on empirical evidence |
| Session resumed | Recreate DAG from session_state (task IDs are unstable) |
| Coverage audit finds gaps | Return to analysis only if `analysis_iteration` < `{MAX_RETURNS}` |
| `analysis_iteration` >= `{MAX_RETURNS}` | Proceed forward regardless |

### Configuration

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `{PHASE_COUNT}` | Number of pipeline phases | 7 | yes |
| `{PHASE_N_NAME}` | Name for each phase | "Review findings" | yes |
| `{GATE_SCRIPT}` | Path to phase gate script | "scripts/review_gate.py" | no |
| `{MAX_RETURNS}` | Maximum returns to earlier phase | 2 | yes |
| `{TASK_PREFIX}` | Phase task prefix format | "[Phase {N}]" | yes |
| `{SUBTASK_PREFIX}` | Subtask prefix format | "[{N}.{M}]" | yes |
| `{STATE_FILE}` | Session state for DAG recreation | "context/session_state.json" | yes |
| `{REQUIRED_ARTIFACTS}` | Files gate checks for existence | ["review_findings/*.json"] | no |

<!-- REFERENCE: load on adoption -->
## Adaptation Guide

### Prerequisites
- A pipeline with 3+ sequential phases where ordering matters
- A task management system that supports `blockedBy` dependencies (or equivalent)
- Filesystem for gate scripts and session state
- At least one phase where skipping has caused problems (empirical evidence)

### Steps to Adopt
1. Define the pipeline phases with sequential ordering and names
2. Create the Task DAG template: one Task per phase with `blockedBy` chains
3. Establish hierarchical naming convention (`[Phase N]` + `[N.M]`)
4. Identify which phase transitions need gate scripts (only those with empirical skip evidence)
5. Implement gate scripts that check for real work products, not just infrastructure files
6. Add `analysis_iteration` counter for bounded feedback loops
7. Define `max_returns` per feedback loop to prevent infinite cycles
8. Implement DAG recreation logic for session resume
9. Add phase naming discipline: use verbs (Review, Merge, Analyze), not nouns (Review, Merge, Analysis)
10. Document the forward-only rule with explicit exceptions

### What to Customize
- Number and names of phases -- match your pipeline's actual structure
- Which transitions get gate scripts -- add only where problems occurred
- Gate check conditions -- what constitutes "real work" varies by domain
- `{MAX_RETURNS}` value -- balance coverage completeness vs. infinite loops
- Subtask naming depth -- `[N.M]` may suffice or `[N.M.K]` may be needed
- Task system -- adapt `blockedBy` to your task management tool's dependency mechanism

### What NOT to Change
- One Task per phase with sequential dependencies -- without this, agents skip phases
- Hierarchical naming with phase prefix -- without this, phases drown in subtasks
- Infrastructure != completion principle -- the single most common agent failure mode
- Gate scripts check real output, not setup files -- setup verification is meaningless
- Forward-only movement (with bounded exceptions) -- unbounded backward movement creates infinite loops
- DAG recreation from session_state on resume -- task IDs are unstable across sessions
- Empirical gate placement -- preemptive gates at every boundary add complexity without value

<!-- HISTORY: load for audit -->
## Origin
- **Source agent:** spec-creator
- **Findings:** [5] Phase gate with Task DAG enforcement, [44] Task DAG for pipeline phase enforcement, [45] Task pollution hides phase tasks, [46] Infrastructure creation != phase completion, [50] Phase gate checks need script enforcement, [65] Review gate as mechanical enforcement (7 conditions), [66] Forward-only pipeline with bounded returns, [73] Gate should check only phases that actually failed
- **Discovered through:** The review phase was skipped twice despite textual pipeline rules. First incident (2026-02-25): context compaction lost phase ordering, agent created YAML cards directly, bypassing review. Second incident (2026-03-03): agent created review_state.json and review_plan.md infrastructure files, then marked the review phase complete without running any reviewer subagents. 30+ subtasks buried the 7 phase tasks, making it impossible to distinguish phase completion from subtask completion. The fix was three-layered: (1) Task DAG with `blockedBy` for mechanical ordering, (2) `[Phase N]` naming to separate phases from subtasks, (3) `review_gate.py` checking 7 conditions including `totals.reviewed > 0`.
- **Evidence:** Two documented phase-skipping incidents resolved. Task DAG setup costs ~2 minutes, prevented multi-hour pipeline violations. Gate script caught the infrastructure-as-completion confusion by verifying `totals.reviewed > 0`. Hierarchical naming made 7 phase tasks visually distinct from 30+ subtasks. Forward-only rule with max 2 returns to analysis prevented infinite coverage audit loops.

## Related Patterns
- [Checkpoint & Resume](checkpoint_resume.md) -- checkpoints record phase position; DAG is recreated from session_state on resume
- [Subagent Coordination](subagent_coordination.md) -- gate scripts verify subagent work was performed, not just dispatched
- [Validation Gates](validation_gates.md) -- gate scripts are a specialized form of validation gates at phase boundaries
- [Autonomous Loop Protocol](loop_protocol.md) -- bounded returns are a controlled form of loop iteration
- [Provider Resilience](provider_resilience.md) -- provider failures within a phase do not bypass the gate
- cognitive_offload -- gate scripts offload phase-completion verification from agent judgment to code
- append_only_audit -- phase completion records are append-only in extraction log
- invariant_testing -- gate conditions are structural invariants that all valid phase completions satisfy
- maturity_metrics -- delta (Process Determinism) metric directly measures how much enforcement is scripted vs. textual
- analysis_review_merge -- the three-phase separation (analysis, review, merge) is enforced by this pattern
- [Knowledge Layer Architecture](../methodologies/knowledge_layers.md) -- phase gates enforce layer boundaries; gate scripts are Layer 0 infrastructure
