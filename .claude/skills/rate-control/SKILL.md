---
name: rate-control
description: "Rate control and token budget management — weekly limits, mode table, efficiency rules, 429 handling. Load when checking budget or hitting rate limits."
user-invocable: false
allowed-tools: Bash, Read
---

# Rate Control

Claude Code has a weekly limit (resets every Friday). This is the primary bottleneck.

## Before Heavy Work

```bash
node ~/.claude/rate-control/claude_limits.mjs --json
```

Read `mode`, then follow:

| Weekly % | Mode | Behavior |
|----------|------|----------|
| < 60% | **Full** | No restrictions |
| 60-75% | **Normal** | Skip exploratory subagents |
| 75-85% | **Light** | Partial reads, batch edits, no re-reads |
| 85-95% | **Eco** | Finish current task only. No subagents. Short answers. Alert operator |
| > 95% | **Stop** | Alert operator. No new tasks until weekly reset |

Re-check every 2h in long sessions.

## Efficiency

- **Don't re-read files** already in context. Batch related edits.
- **Grep before Read.** Find lines first, read only those.
- **Output costs 5x input.** Be concise. Don't restate what user sees.
- **Edit > Write.** Edit sends diff only. Write sends whole file.
- **Subagents for research.** Results return as summary, not full context.

## On 429

1. Stop immediately. Do not retry.
2. Run `node ~/.claude/rate-control/claude_limits.mjs` to identify which window.
3. Report status to operator.
4. Session window: wait for reset. Weekly: eco mode until reset (see `days_left`).

## Daily Budget

The dashboard computes `daily_budget_usd` automatically — how much you can spend today to last until weekly reset.
