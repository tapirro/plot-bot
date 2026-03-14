---
id: cross-human-review-digest
title: Human Review Digest
type: pattern
concern: [human-feedback]
mechanism: [pipeline, scoring-model]
scope: per-item
lifecycle: [act, reflect]
origin: harvest/voic-experiment
origin_findings: [22, 51]
maturity: draft
domain: cross-agent/patterns
confidence: observed
basis: "distilled from voic-experiment harvest, 65 findings from voice agent sessions"
---

# Human Review Digest

<!-- CORE: load always -->
## Problem

Agent-to-human handoff is unstructured. When an agent completes a cycle of work, the human reviewer receives either a changelog (too technical, gets ignored) or a vague summary (too abstract, provides no basis for evaluation). The human does not know what to test, in what order, or what a successful outcome looks like.

Free-text feedback, when it arrives, cannot be processed systematically. An agent cannot distinguish "this is broken" from "this could be better" when both arrive as prose paragraphs. The feedback sits in a document, disconnected from the backlog, and never influences what the agent works on next.

Untested items accumulate silently. Without explicit tracking, neither the agent nor the human knows which changes were actually verified and which were simply assumed to be working. Over multiple cycles, confidence in the system erodes because no one can point to evidence that specific changes were validated by a human.

## Solution

A machine-readable digest translates each change into a testable claim with numbered verification steps. Each item states an observable expectation -- what the human should notice -- and provides concrete steps to verify it. This eliminates ambiguity about what to test and how.

Humans score each item on a fixed 1-5 scale rather than providing free-text-only feedback. The numeric scale enables systematic processing: a conversion script maps scores to actionable statuses (fix required, validated, carry forward) that feed directly into the next cycle's backlog. Score 1-2 creates a prioritized fix item; score 4-5 validates the change; score 3 is noted without action.

Items not tested are explicitly tracked as "carry forward" rather than silently dropped. If an item remains untested for a configurable number of consecutive cycles, it escalates automatically, forcing the reviewer to either test it or consciously skip it.

The digest includes a validation gate that checks structural completeness before presentation: every item must have an observable expected claim, numbered test steps, and a link to a north star metric. Malformed digests are blocked, preventing low-quality handoffs that waste reviewer time. The entire pipeline -- from digest generation through scoring to backlog update -- forms a closed loop where human judgment directly shapes agent priorities.

## Implementation

### Structure

```
REVIEW_DIGEST
├── cycle_id: int
├── generated_at: datetime
├── items:
│   ├── ITEM
│   │   ├── id: string                    ← backlog item ID (e.g., S-A.5, AB-012)
│   │   ├── title: string                 ← human-readable title
│   │   ├── summary: string               ← 2-3 sentences: what changed and why
│   │   ├── north_star: string            ← which key metric this relates to
│   │   ├── expected: string              ← observable claim (what the human should notice)
│   │   ├── how_to_test: list of string   ← numbered steps to verify
│   │   ├── test_config: object | null    ← machine-readable overrides (optional)
│   │   └── feedback: object | null       ← filled by human
│   │       ├── score: int (1-5)
│   │       └── comment: string | null
│   └── ...
└── summary: object | null                 ← filled by conversion script
```

### Item Fields

**id:** The backlog item identifier, linking the digest item back to the originating work item. Uses the same ID scheme as the backlog (e.g., `[S-{LETTER}.{PHASE}]`, `[AB-{NNN}]`, `[R-{NN}:{REF_ID}]`).

**title:** A concise human-readable name. Not technical — written for the person who will test it.

**summary:** 2-3 sentences explaining what was changed and why. Focus on the user-visible effect, not implementation details.

**north_star:** A code or label linking this change to a tracked metric. Enables aggregation: "3 items this cycle targeted `{METRIC_A}`."

**expected:** An observable claim that the human can verify without technical knowledge. Written as a prediction:
- "Processing should complete within `{EXPECTED_DURATION}` instead of `{PREVIOUS_DURATION}`"
- "The output for `{TEST_INPUT}` should now include `{EXPECTED_ELEMENT}`"
- "Error rate on `{SCENARIO}` should drop below `{THRESHOLD}`"

**how_to_test:** Numbered steps. Each step is a concrete action, not a vague instruction.

```
1. Navigate to {LOCATION}
2. Trigger {ACTION} with {INPUT}
3. Observe {WHAT_TO_LOOK_AT}
4. Verify that {EXPECTED_OUTCOME}
```

Rules for test steps:
- Maximum `{MAX_STEPS}` steps per item
- Each step starts with a verb
- No step requires technical knowledge beyond what the human role implies
- If special configuration is needed, provide it in `test_config`

**test_config:** Machine-readable overrides that can be auto-applied to create the testing environment. Optional — only needed when the default configuration does not expose the change.

```json
{
  "parameter_overrides": {
    "{PARAM_A}": "{VALUE_A}",
    "{PARAM_B}": "{VALUE_B}"
  },
  "test_data": "{TEST_DATA_REF}",
  "environment": "{ENV_NAME}"
}
```

### Feedback Scale

| Score | Meaning | Agent Interpretation |
|-------|---------|---------------------|
| 1 | Broken — does not work at all | Fix required, P0 |
| 2 | Poor — partially works but below expectations | Fix required, P1 |
| 3 | Acceptable — works but not impressive | Noted, may revisit |
| 4 | Good — meets expectations | Validated |
| 5 | Excellent — exceeds expectations | Validated, reference for future work |

### Conversion Rules

A script (`{REVIEW_SCRIPT}`) processes the completed digest and produces actionable statuses:

```
FOR each item IN digest.items:
    IF item.feedback IS null:
        status = "carry_forward"
        reason = "untested — carry to next digest"

    ELSE IF item.feedback.score <= 2:
        status = "fix_required"
        priority = 6 - item.feedback.score    ← score 1 → P0, score 2 → P1
        reason = item.feedback.comment OR "low score, no comment"

    ELSE IF item.feedback.score >= 4:
        status = "validated"
        reason = item.feedback.comment OR "score >= 4"

    ELSE:  ← score = 3
        status = "noted"
        reason = item.feedback.comment OR "acceptable, no action required"

    EMIT {id, status, priority, reason, north_star}
```

### Digest Validation Gate

Before presenting the digest to a human, validate:

| Check | Rule | On Failure |
|-------|------|-----------|
| Required keys | Every item has: id, title, summary, expected, how_to_test | Block digest, fix missing fields |
| Numbered steps | `how_to_test` has at least 1 and at most `{MAX_STEPS}` numbered steps | Block digest, fix step count |
| Observable expected | `expected` contains a measurable or observable claim | Warning, review manually |
| North star present | `north_star` is not empty | Warning, assign `"unlinked"` |
| No duplicates | No two items share the same `id` | Block digest, deduplicate |

### Feedback-to-Backlog Pipeline

```
DIGEST (human fills feedback)
  → CONVERSION SCRIPT (scores → statuses)
    → BACKLOG UPDATE:
        fix_required → new auto-backlog item [AB-{NNN}] with priority
        validated → mark original item as done, record impact
        carry_forward → re-include in next cycle's digest
        noted → close, no further action unless pattern emerges
```

If the same item receives `carry_forward` for `{MAX_CARRY}` consecutive cycles, escalate to the human with a note: "This item has been untested for `{MAX_CARRY}` cycles. Please test or explicitly skip."

### Configuration

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `{MAX_STEPS}` | Maximum test steps per item | 5 | yes |
| `{MAX_CARRY}` | Cycles before untested item escalates | 3 | yes |
| `{REVIEW_SCRIPT}` | Name of conversion script | review_summary.py | yes |
| `{METRIC_A}` | Example north star metric | "processing_latency" | context-dependent |
| `{EXPECTED_DURATION}` | Example expected value | "2s" | context-dependent |
| `{ENV_NAME}` | Testing environment name | "staging" | no |
| `{TEST_DATA_REF}` | Reference to test data | "fixtures/sample_input.json" | no |

<!-- REFERENCE: load on adoption -->
## Adaptation Guide

### Prerequisites
- A backlog system with item IDs (see Three-Tier Backlog Management)
- A human reviewer with a defined role and availability cadence
- A mechanism to present the digest (file, web UI, message)
- A script runner to execute the conversion rules

### Steps to Adopt
1. Define your digest template with all required fields (id, title, summary, north_star, expected, how_to_test)
2. At the end of each cycle, generate one digest item per completed work item
3. Write `expected` as an observable claim, not a technical description
4. Write `how_to_test` as numbered steps starting with verbs
5. Optionally add `test_config` for items requiring special setup
6. Present digest to human reviewer
7. Human fills in `score` (1-5) and optional `comment` per item
8. Run conversion script to map scores to statuses
9. Feed statuses back into backlog: fix_required creates new items, validated closes items
10. Carry forward untested items to the next digest
11. Escalate items that remain untested for `{MAX_CARRY}` cycles

### What to Customize
- North star codes (match your metric taxonomy)
- Test step style (adapt to your reviewer's technical level)
- Feedback scale descriptions (calibrate to your quality expectations)
- Conversion thresholds (which scores map to which statuses)
- test_config format (match your system's configuration mechanism)
- Carry-forward escalation threshold
- Digest delivery mechanism (file, Slack, email, web UI)

### What NOT to Change
- Observable `expected` field — without it, humans do not know what to look for
- Numbered `how_to_test` steps — unstructured instructions produce unstructured feedback
- Fixed 1-5 scale — free-text-only feedback cannot be systematically processed
- Conversion script producing actionable statuses — the pipeline must close the loop
- Carry-forward tracking — untested items must not silently disappear
- Validation gate before presenting to human — malformed digests waste reviewer time
- Feedback flowing back to backlog — without this, human opinions are decorative

<!-- HISTORY: load for audit -->
## Origin
- **Source agent:** voic-experiment
- **Findings:** [22] Per-Feature Human Review Digest, [51] Digest Validation Gate and Conversion Pipeline
- **Discovered through:** iterative development of human-in-the-loop feedback. Early versions provided changelogs that humans ignored. Switching to observable claims with numbered test steps increased feedback completion. The conversion script was added after noticing that free-text feedback could not be systematically processed. The carry-forward mechanism was added after items went untested for multiple cycles without anyone noticing. The validation gate was added after malformed digests (missing test steps, vague expected claims) led to low-quality feedback.
- **Evidence:** Feedback completion rate increased after switching from changelogs to structured digests. Score-to-status conversion enabled automated backlog updates. Carry-forward tracking surfaced 3 items that had been silently ignored for multiple cycles.

## Related Patterns
- [Three-Tier Backlog Management](../methodologies/backlog_management.md) — fix_required items feed into auto-backlog, validated items close backlog items
- [Hypothesis-Driven Experimentation](../methodologies/experimentation.md) — human feedback complements automated verification
- [Autonomous Loop Protocol](../patterns/loop_protocol.md) — digest is generated at the REPORT phase of each cycle
- [Observability Engine](../patterns/observability_engine.md) — provides data for the `expected` claims and before/after comparisons
