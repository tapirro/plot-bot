---
id: cross-validation-gates
title: Validation Gates
type: pattern
concern: [validation-gating]
mechanism: [gate-pyramid, state-machine]
scope: per-cycle
lifecycle: [decide]
origin: harvest/multi
origin_findings:
  hilart-ops-bot: [9, 11]
  voic-experiment: [18, 46, 24, 40]
  spec-creator: [1, 26]
maturity: draft
domain: cross-agent/patterns
confidence: observed
basis: "cross-agent pattern from hilart-ops-bot, voic-experiment, and spec-creator harvests"
---

# Validation Gates

<!-- CORE: load always -->
## Problem

Sequential agent decisions -- detect, classify, report, escalate -- cascade errors silently through the pipeline. A false detection produces a misclassified incident, which generates a spurious report, which triggers an unnecessary escalation. Each stage trusts the output of the previous stage without verification, so a single bad input propagates through the entire chain before anyone notices.

In automated feedback loops with fix-and-verify cycles, a more subtle failure mode emerges: regression oscillation. The agent fixes failure A, but the fix breaks test B. Next loop, it fixes B, which re-breaks A. The agent cycles between these two states indefinitely, consuming resources without converging. Without a mechanism to require sustained resolution, the agent cannot distinguish between a genuine fix and a lucky pass.

At scale, these problems interact. Cascading errors generate false incidents that bloat the backlog, which triggers more fix attempts, which cause more regressions. Circuit-breaking mechanisms are needed to prevent runaway loops from exhausting the agent's entire cycle on a single cluster of oscillating failures.

## Solution

Two complementary mechanisms address different failure modes.

**Static gate pyramid** places boolean validation checks at phase transition points in the pipeline. Each gate verifies preconditions before allowing the next phase to proceed. Gates are layered by concern: structural checks (state file exists, parses correctly), logical checks (severity justified by data, no orphan references), and operational checks (rate limits respected, no duplicate escalations in window). A gate failure blocks the transition, preventing bad data from propagating downstream.

**Anti-flap state machine** tracks each item (failure, incident, backlog entry) across runs through four states: ACTIVE, TENTATIVE, RESOLVED, and STALE. An item is not considered fixed until it passes a configurable number of consecutive verification runs (TENTATIVE phase). Even after reaching RESOLVED, the item remains under watch for additional runs to catch regressions. Items not observed for a staleness threshold are archived as STALE -- distinct from RESOLVED, because absence of failure is not the same as confirmed fix.

Circuit breakers prevent runaway behavior: maximum active items, fix attempt escalation ladders (auto-fix, then guided fix, then BLOCKED), cascade detection for simultaneous P1 failures, and cost caps. UNK records convert hallucination risk into trackable work items -- when a gate encounters a required field without sufficient evidence, it creates an explicit unknown record rather than allowing the agent to invent a plausible value.

## Implementation

### Structure

```
GATE
├── id: string
├── name: string
├── when: string          # trigger point in the loop
├── checks: list[CHECK]
├── on_fail: BLOCK | WARN
└── last_result: PASS | FAIL
```

```
TRACKED_ITEM
├── id: string            # dedup key from taxonomy
├── state: ACTIVE | TENTATIVE | RESOLVED | STALE
├── consecutive_passes: int
├── fix_attempts: int
├── first_seen_run: int
├── last_seen_run: int
├── last_failure_detail: object
└── history: list[RUN_RESULT]
```

### UNK Record (Unknown Tracking)

```
UNK_RECORD
├── id: "UNK-{SEQUENCE}"
├── artifact_id: string              # which artifact has the gap
├── field: string                    # which field is unknown
├── reason: string                   # why it's unknown (insufficient evidence, ambiguous source, etc.)
├── owner: string                    # who should resolve (stakeholder, analyst, domain expert)
├── status: open | resolved
├── created_at: datetime
└── resolved_value: any | null       # filled when resolved; null while open
```

UNK records convert hallucination risk into trackable work items. When a gate encounters a required field without sufficient evidence, the field is not invented -- it becomes a UNK record that persists until explicitly resolved.

### Gate Pyramid (5 Layers)

| Layer | Gate | When | Key Checks | On Fail |
|-------|------|------|------------|---------|
| 1 | **pre-loop** | Before every loop | State valid, data sources online, phase = IDLE | BLOCK |
| 2 | **pre-report** | Before generating report | Incidents have valid fields, severity justified by data | BLOCK |
| 3 | **pre-escalation** | Before sending escalation | Severity >= threshold, rate limit ok, no duplicate in window | BLOCK |
| 4 | **pre-taxonomy** | Before adding new type | 3+ observations, distinct from existing types | BLOCK |
| 5 | **pre-commit** | Before committing changes | Repo structure valid, no secrets, syntax checks pass | BLOCK |

### Gate Checks (Examples)

**pre-loop:**
- `state.json` exists and parses correctly
- All required data sources respond
- Current phase is IDLE (not mid-loop)
- Loop counter is sane (not negative, not impossibly large)

**pre-report:**
- Every incident has: type, severity, confidence, timestamp
- Severity is justified: threshold deviation matches claimed severity level
- No orphan investigations (investigation exists -> parent incident exists)

**pre-escalation:**
- Severity >= `{MIN_ESCALATION_SEVERITY}`
- Confidence >= `{ESCALATION_THRESHOLD}`
- Rate limit: < `{MAX_ESCALATIONS}` in last hour
- No duplicate of same incident type + unit in last `{DEDUP_HOURS}` hours

**pre-taxonomy:**
- `{MIN_OBSERVATIONS}` similar incidents observed
- Proposed type doesn't overlap with existing types
- Naming follows `{UNIT}-{CATEGORY}-{DETAIL}` convention

**pre-commit:**
- Repository structure validation passes
- No secrets or credentials in changed files
- Query syntax valid (if applicable)

### Anti-Flap State Machine

Prevents regression oscillation in automated feedback loops. Each tracked item (failure, incident, backlog entry) moves through states based on consecutive test/verification results:

```
                    fail
        ┌───────────────────────────┐
        v                           │
     ACTIVE ──── pass ──── TENTATIVE ──── {REQUIRED_PASSES} consecutive passes ──── RESOLVED
        ^                                                                               │
        │                                                                               │
        └──────────────────── fail (within {WATCH_WINDOW} runs) ────────────────────────┘
```

| State | Meaning | Transition |
|-------|---------|------------|
| ACTIVE | Item is failing | On pass -> TENTATIVE (reset consecutive_passes = 1) |
| TENTATIVE | Passed but not yet stable | On pass -> increment consecutive_passes. If >= `{REQUIRED_PASSES}` -> RESOLVED. On fail -> ACTIVE |
| RESOLVED | Sustained fix confirmed | On fail within `{WATCH_WINDOW}` runs -> ACTIVE (regression!). After `{WATCH_WINDOW}` -> remove from tracking |
| STALE | Not seen for `{STALENESS_THRESHOLD}` runs | Auto-transition from ACTIVE if `current_run - last_seen_run > {STALENESS_THRESHOLD}` |

**Key properties:**
- An item is not "fixed" until it passes `{REQUIRED_PASSES}` consecutive times
- RESOLVED items remain in watch for `{WATCH_WINDOW}` additional runs to catch regressions
- STALE detection prevents ghost items from accumulating (items not seen for `{STALENESS_THRESHOLD}`+ runs are archived)

### Circuit Breakers

Circuit breakers prevent runaway loops and resource exhaustion:

| Breaker | Condition | Action |
|---------|-----------|--------|
| Max active items | ACTIVE items > `{MAX_ACTIVE_ITEMS}` | Pause new detection, focus on fixes |
| Fix attempt escalation | `fix_attempts` > `{MAX_FIX_ATTEMPTS}` for any item | Escalate to owner, stop auto-fixing |
| Cascade detection | `{CASCADE_THRESHOLD}`+ P1 items simultaneously ACTIVE | Emergency stop, full diagnostic |
| Cost cap | Cycle cost > `{COST_CAP}` | Skip optional operations (research, BOLD loops) |

**Fix attempt escalation ladder:**

| Attempts | Action |
|----------|--------|
| 1-`{AUTO_FIX_LIMIT}` | Agent attempts automated fix |
| `{AUTO_FIX_LIMIT}`+1 - `{ASSISTED_FIX_LIMIT}` | Agent requests owner guidance |
| > `{ASSISTED_FIX_LIMIT}` | Item marked as BLOCKED, excluded from auto-fix |

### Staleness Detection

Items in ACTIVE state that are not observed in subsequent runs may indicate:
- The test/check that detected them was removed
- The code path that triggers them no longer executes
- Environmental changes made the failure irreproducible

```
IF current_run - item.last_seen_run > {STALENESS_THRESHOLD}
THEN item.state = STALE
  -> Archive with reason "not observed for {STALENESS_THRESHOLD}+ runs"
  -> Do NOT count as RESOLVED (no fix was confirmed)
  -> Log for periodic review
```

### Test Pyramid for State Machine

The anti-flap state machine is safety-critical logic. It requires thorough testing:

| Test Layer | Count Target | Covers |
|------------|-------------|--------|
| Pure function unit tests | `{STATE_MACHINE_UNIT_TESTS}`+ | All state transitions, edge cases, boundary conditions |
| Property-based tests | `{PROPERTY_TESTS}`+ | Invariants: no item can skip states, consecutive_passes monotonic within TENTATIVE |
| Integration tests | `{INTEGRATION_TESTS}`+ | State persistence across runs, dedup key matching |

**Critical test cases:**
- ACTIVE -> TENTATIVE -> ACTIVE (single pass then fail = reset)
- ACTIVE -> TENTATIVE -> ... -> RESOLVED (exactly `{REQUIRED_PASSES}` passes)
- RESOLVED -> ACTIVE (regression within watch window)
- RESOLVED -> removed (no regression after watch window)
- ACTIVE -> STALE (not seen for `{STALENESS_THRESHOLD}`+ runs)
- Concurrent items: one RESOLVED while another ACTIVE (independence)
- Dedup key collision: same key, different runs (update vs create)

### Validation Runner

A master script that runs appropriate gates:

```bash
# Run all gates for a loop phase
gate_runner.sh {gate_name}

# Run all structural validation
gate_runner.sh all

# Check result
exit code 0 = PASS, non-zero = FAIL with reason on stderr
```

### Configuration

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `{MIN_ESCALATION_SEVERITY}` | Minimum severity to escalate | HIGH | yes |
| `{ESCALATION_THRESHOLD}` | Minimum confidence for escalation | 0.6 | yes |
| `{MAX_ESCALATIONS}` | Max escalations per hour | 5 | yes |
| `{DEDUP_HOURS}` | Deduplication window | 4 | yes |
| `{MIN_OBSERVATIONS}` | Observations needed for new type | 3 | yes |
| `{REQUIRED_PASSES}` | Consecutive passes to confirm fix | 3 | yes |
| `{WATCH_WINDOW}` | Runs to watch after RESOLVED | 5 | yes |
| `{STALENESS_THRESHOLD}` | Runs without observation before STALE | 10 | yes |
| `{MAX_ACTIVE_ITEMS}` | Circuit breaker: max active failures | 20 | yes |
| `{MAX_FIX_ATTEMPTS}` | Circuit breaker: max auto-fix per item | 5 | yes |
| `{CASCADE_THRESHOLD}` | Circuit breaker: simultaneous P1 count | 3 | yes |
| `{COST_CAP}` | Circuit breaker: max cycle cost | Configurable | no |
| `{AUTO_FIX_LIMIT}` | Attempts before requesting guidance | 3 | yes |
| `{ASSISTED_FIX_LIMIT}` | Attempts before marking BLOCKED | 5 | yes |
| `{STATE_MACHINE_UNIT_TESTS}` | Min unit tests for state machine | 49 | no |
| `{PROPERTY_TESTS}` | Min property-based tests | 10 | no |
| `{INTEGRATION_TESTS}` | Min integration tests | 5 | no |

### Decision Rules

- **BLOCK vs WARN**: Use BLOCK for gates where proceeding on failure would cause cascading errors (pre-loop, pre-escalation). Use WARN for advisory gates where the agent can proceed with caution (quality gate post-commit).
- **Required passes**: Start with `{REQUIRED_PASSES}` = 3. Increase if operating in noisy environments where false passes are common.
- **Staleness threshold**: Set to `{STALENESS_THRESHOLD}` = 10 runs. Shorter values risk premature archival. Longer values accumulate ghost items.
- **Circuit breaker tuning**: `{MAX_ACTIVE_ITEMS}` should be 2-3x the typical active count. Too tight = frequent pauses. Too loose = resource exhaustion.
- **Fix escalation**: `{AUTO_FIX_LIMIT}` should be low (2-3). Automated fixes that don't work after 3 attempts are unlikely to work on attempt 4.
- **UNK over hallucination**: If a required field has insufficient evidence, DO NOT invent a value. Create a UNK record with artifact_id, field, and reason. Set the artifact field to "See UNK-{ID}". The UNK becomes a trackable work item resolved by a human or a subsequent pipeline run with better sources. This rule applies at every gate that validates artifact content.

<!-- REFERENCE: load on adoption -->
## Adaptation Guide

### Prerequisites
- A loop protocol with defined phases
- Incident entity model with severity and confidence
- File system or state mechanism for gate results
- For anti-flap: a test/verification pipeline that runs on each loop
- For circuit breakers: defined cost model (even approximate)

### Steps to Adopt

**Static gates (start here):**
1. Start with **pre-loop** and **pre-escalation** gates only (highest ROI)
2. Add **pre-report** when false reports become a problem
3. Add **pre-commit** when the agent modifies its own repository
4. Add **pre-taxonomy** when the agent can propose new incident types
5. Implement as simple functions/scripts that return pass/fail
6. Log gate results for debugging (which check failed and why)

**Anti-flap state machine (add when oscillation detected):**
7. Implement TRACKED_ITEM entity with state field
8. Start with ACTIVE and RESOLVED states only (simplest version)
9. Add TENTATIVE state with `{REQUIRED_PASSES}` = 3
10. Add STALE detection
11. Write `{STATE_MACHINE_UNIT_TESTS}`+ pure-function unit tests covering all transitions
12. Add state persistence across runs

**Circuit breakers (add when operating at scale):**
13. Implement `{MAX_ACTIVE_ITEMS}` breaker first (prevents overwhelm)
14. Add fix attempt escalation ladder
15. Add cascade detection
16. Add cost cap if applicable

### What to Customize
- Specific checks per gate (depends on your data sources, state format)
- Thresholds (escalation severity, rate limits, dedup window)
- Which gates exist (start with 2, grow to 5)
- `{REQUIRED_PASSES}` count (depends on environment noise)
- `{STALENESS_THRESHOLD}` (depends on run frequency)
- Circuit breaker limits (depends on agent capacity)
- Fix escalation ladder (depends on owner availability)

### What NOT to Change
- Gate = boolean check at transition point (pass/fail, not scoring)
- Failure BLOCKS the transition (not just warns) for critical gates
- Gates are layered: structural -> logical -> operational
- Every gate logs its result (pass or fail + reason)
- pre-loop and pre-escalation are mandatory for any monitoring agent
- Anti-flap requires TENTATIVE state (don't go directly from ACTIVE to RESOLVED)
- STALE is distinct from RESOLVED (absence of failure != confirmed fix)
- Circuit breakers must exist for any agent with auto-fix capability
- State machine must have comprehensive unit tests (safety-critical logic)

<!-- HISTORY: load for audit -->
## Origin

### hilart-ops-bot
- **Findings:** [9] Validation Gates (5-layer pyramid), [11] 5-Layer Validation Pyramid
- **Discovered through:** Pre-flight checks (v0.4.0) were added after cascading errors from invalid state caused false detections. Each layer was added in response to a specific failure mode: invalid state (pre-loop), unjustified severity (pre-report), false escalation (pre-escalation), taxonomy bloat (pre-taxonomy), broken repo (pre-commit).
- **Evidence:** Pre-flight checks prevented cascading errors. Rate limiting and deduplication in pre-escalation gate prevented alert fatigue.

### voic-experiment
- **Findings:** [18] Anti-Flap State Machine, [46] Circuit Breakers, [24] Staleness Detection, [40] State Machine Test Pyramid
- **Discovered through:** 226 loops with automated fix-and-verify pipeline. Early versions had no anti-flap protection, causing the same failure to oscillate between fixed and broken for 10+ consecutive loops. The TENTATIVE state (requiring 3 consecutive passes) eliminated oscillation. Circuit breakers added after a cascade of 5 P1 failures consumed an entire cycle without progress. Staleness detection added after discovering 15+ phantom items in the backlog that hadn't been observed in 10+ runs. 49 pure-function unit tests written for the state machine to ensure correctness.
- **Evidence:** Anti-flap reduced regression oscillation from ~15% of loops to < 2%. Circuit breakers prevented 3 runaway cycles. Staleness detection cleaned 15 phantom items from backlog. 49 unit tests caught 4 edge-case bugs during development.

### spec-creator
- **Findings:** [1, 26] No-Hallucination Rule with Explicit Unknown Tracking
- **Discovered through:** Building a specification extraction pipeline that processes source code into structured artifacts. When agents encountered required fields without sufficient evidence, they invented plausible-looking content rather than leaving gaps. The UNK tracking pattern converts each missing value into a structured record (artifact_id, field, reason, owner) that persists as a trackable work item.
- **Evidence:** Across 16+ extraction runs, UNK tracking eliminated hallucination in required fields. Every missing value becomes a trackable work item rather than an invented fact. Combined with the append-only audit trail, UNK records provide full traceability of what was known vs. unknown at each pipeline stage.

## Related Patterns
- [Autonomous Loop Protocol](../patterns/loop_protocol.md) -- gates check at loop phase transitions
- [Confidence & Severity Model](../methodologies/confidence_severity.md) -- pre-escalation checks confidence
- [Incident Taxonomy](../patterns/incident_taxonomy.md) -- pre-taxonomy gate controls evolution
- [Closed-Loop Quality System](../methodologies/closed_loop_quality.md) -- quality pipeline produces items tracked by anti-flap state machine
- [Provider Resilience](../patterns/provider_resilience.md) -- pre-request gates can check provider health
- [Three-Tier Backlog Management](../methodologies/backlog_management.md) -- gate failures generate auto-backlog items
- [Scenario-First Testing](../methodologies/scenario_first_testing.md) -- quality gate is a specialized validation gate
- [Artifact-Centric Interface](../patterns/artifact_centric_interface.md) -- gates validate file schema before commits
- [Invariant-Based Testing](../methodologies/invariant_testing.md) -- groundedness invariant verifies no hallucinated fields
- [Append-Only Audit Trail](../patterns/append_only_audit.md) -- UNK records follow append-only policy
- [Analysis-Review-Merge Pipeline](../methodologies/analysis_review_merge.md) -- review gate is a specialized validation gate for phase transitions
- [Adversarial Reflection](../patterns/adversarial_reflection.md) -- adversarial review can strengthen gate check quality
- [Cognitive Offload](../methodologies/cognitive_offload.md) -- gate scripts replace manual validation, offloading mechanical checks
- [Context Budget Management](../methodologies/context_budget.md) -- gate documents are GATE-tier loading priority
- [Maturity Metrics](../methodologies/maturity_metrics.md) -- gate pass rates contribute to iota measurement
- [Repository-as-Product (RaP)](../methodologies/rap_methodology.md) -- gate scripts are Layer 0 algorithmization of text rules
