---
id: pattern-agent-tool-design
type: pattern
status: active
domain: plot-bot
tags: [agent-tooling, token-economics, design-patterns]
origin: self
confidence: validated
basis: "9 tool design patterns extracted from 50+ tool iterations"
---

# Agent-First Tool Design

How to build CLI tools that serve AI agents as primary consumers while remaining useful for humans and APIs.

## Problem

AI agents interact with tools hundreds of times per session. Every tool invocation costs tokens — input (reading output) and context (carrying output forward). Traditional CLI tools are designed for humans: verbose output, decorative formatting, implicit knowledge of flags. When an agent uses such tools, it wastes tokens on noise, needs extensive upfront documentation of all flags, and can't discover next actions from output alone.

Empirical data from 112 sessions (see `agent_token_economics.md`) shows that orientation reads and tool output processing consume 46% of total context budget. A tool that returns 50 lines when 5 would suffice costs not just the extra 45 lines once — it costs them on every subsequent turn as context accumulates. Meanwhile, front-loading all tool documentation into system prompts wastes 300-500 tokens per turn on instructions the agent may never need. The core tension: agents need tools that are both self-documenting (zero upfront cost) and minimal (zero ongoing cost).

## Solution

Design tools where the **agent is the first-class client**. Optimize for token cost, cognitive simplicity, and progressive context delivery. Humans and APIs are served through output mode flags, not by compromising the agent-default path.

The approach rests on nine interlocking principles: (1) agent-default output that minimizes tokens with no flags needed; (2) one-token naming with short aliases to reduce per-invocation cost; (3) three output tiers (agent/human/API) plus a quiet modifier; (4) progressive context via tool-embedded hints that teach the agent at point-of-use instead of front-loading documentation; (5) sub-50ms response time for frequently called tools; (6) named views that encode institutional knowledge as single-token presets; (7) composite commands that collapse multi-step diagnostics into one call; (8) semantic block extraction by role rather than heading text; (9) interactive decision points that replace text lists with structured selection UI.

The key insight is that these principles compound: a tool with one-token aliases, conditional hints, and quiet mode costs 5-10x fewer tokens than a traditional CLI — not through any single optimization but through systematic elimination of waste at every layer of interaction.

## Nine Design Principles

### 1. Agent-Default Output

The default output (no flags) is optimized for agent consumption: minimal tokens, structured enough to parse, no decoration.

```
# Agent default — 2 tokens per line + footer
insight-dtk-0302	knowledge/transcripts/insights/...
insight-dtk-0303	knowledge/transcripts/insights/...
4 items

# Human flag (-w) — formatted table
TYPE     STATUS    UPDATED  TAGS        TITLE
insight  verified  03-04    hilart,dtk  Daily DTK Insights
4 items

# API flag (-j) — JSON contract
{"items":[{"id":"insight-dtk-0302","type":"insight",...}],"total":4}
```

Three output tiers, one codebase. The agent path costs 5-10x fewer tokens than the human path.

**Rule:** If you're choosing between "looks nice" and "fewer tokens", choose fewer tokens for the default. The `-w` flag exists for when appearance matters.

### 2. One-Token Naming + Short Aliases

Every command and flag should have two forms: a **discoverable long name** (1 BPE token, English word) and a **compact short alias** (1 character).

| Long (discoverable) | Short (compact) | Bad (multi-token) |
|---------------------|-----------------|-------------------|
| `list` | `l` | `search-artifacts` |
| `lint` | — | `validate` |
| `--type` | `-t` | `--artifact-type` |
| `--past` | `-p` | `--since-date` |
| `--fields` | `-f` | `--output-fields` |
| `--count` | `-c` | `--count-only` |

**Why two forms:** Long names are self-documenting — an agent (or human) encountering the tool for the first time understands `list --type insight`. Short aliases optimize for repeated use — after the agent has learned the tool, `l -t insight` saves 4 tokens per call. Over hundreds of invocations, this compounds.

**Heuristics:**
- Long names: common English word, ≤6 lowercase letters = 1 BPE token
- Short aliases: single letter, mnemonic (`-t` = type, `-f` = fields, `-S` = sort)
- Commands: first letter of long name (`list`→`l`, `get`→`g`, `map`→`m`)
- Avoid collisions: uppercase for disambiguation (`-S` sort vs `-s` schema)

### 3. Output Tiers + Quiet Mode

Every command supports three output modes plus a cross-cutting quiet modifier:

| Mode | Flag | Consumer | Design goal |
|------|------|----------|-------------|
| Agent | *(default)* | AI agent | Minimal tokens, parseable, with hints |
| Human | `-w` | Human operator | Aligned tables, readable formatting |
| API | `-j` | Dashboard/scripts | Compact JSON, stable contract |
| Quiet | `-q` | Agent (experienced) | Suppress hints, footers, and counts |

**Rules:**
- Hints appear in Agent and Human modes. Never in `-j`. Suppressed by `-q`.
- `-q` is the natural endpoint of progressive learning: once the agent knows the tool, hints become noise. The agent opts out when ready.
- Human mode may include data the agent mode omits (edges, full paths, verbose details) — the cost is justified because a human is reading once, not carrying in context.
- API mode is the contract. Breaking changes here break downstream consumers. Agent and human modes can evolve freely.

### 4. Progressive Context via Hints

Instead of front-loading all tool documentation into the agent's system prompt, embed contextual guidance directly into tool output. The agent receives instructions **at the moment they're relevant**, not upfront.

This is the core innovation. See the dedicated section below.

### 5. Speed as Feature

An agent tool that takes 2 seconds to respond costs the agent 2 seconds of wall time × every invocation. For tools called 50+ times per session, startup time dominates.

| Target | Method |
|--------|--------|
| < 10ms for queries | Pre-built index, in-memory filter |
| < 200ms for full scan | Batch I/O, parallel processing |
| 0ms startup | Compiled binary (Go, Rust), not interpreted (Python, Node) |

**Heuristic:** If the tool is invoked more than 10 times per session, it must start in under 50ms. If under 10 times, 500ms is acceptable.

### 6. Named Views (Pre-Composed Workflows)

Agents repeat the same multi-flag queries. Instead of memorizing `list --gap --old --sort updated`, provide named presets: `@stale`, `@health`, `@orphans`.

```
# Without views — agent must remember flag combinations:
./tool list --gap --tier 0 --sort updated
./tool lint --gap && ./tool list --old && ./tool sum

# With views — one token invokes the workflow:
./tool @tier0
./tool @health
```

**Rules:**
- View name = `@` + single English word. Discoverable via `./tool @` (list all views).
- A view is a shortcut, not a separate code path. It expands to the same flags the agent could type manually.
- Views encode **institutional knowledge** — common diagnostic workflows, operator preferences, team conventions.
- Views are where the tool teaches the agent "what questions to ask", not just "how to ask them."

**When to create a view:** When the agent (or operator) runs the same 3+ flag combination for the third time. Same "rule of three" as for scripts.

### 7. Composite Commands (Reduce Round-Trips)

Each tool invocation costs: subprocess spawn + output parsing + context carry. A "health check" that requires `scan` + `lint --gap` + `list --old` + `sum` = 4 calls = 4× overhead.

Composite commands collapse common multi-step diagnostics into one call with one output.

```
# Before: 4 calls, 4 outputs in context
./tool scan          # rebuild index
./tool lint --gap    # find compliance gaps
./tool list --old    # find stale files
./tool sum           # aggregate counters

# After: 1 call, 1 compact output
./tool health
# 329 artifacts, compliance:99%, 12 stale, 1 gap, 0 errors
# hint: 12 stale items. try: ./tool @stale
```

**Rules:**
- A composite replaces a sequence the agent runs >80% of sessions.
- Output is a digest, not concatenation. Don't dump 4 outputs end-to-end.
- The composite still emits hints pointing to drill-down commands.

### 8. Semantic Block Extraction

Agents don't read files top-to-bottom. They need specific blocks: "the decisions from this meeting", "the solution from this pattern", "the action items from this insight."

Instead of requiring the agent to Read a file + find the right section by heading, the tool extracts content by **semantic role**.

```
# Positional extraction (fragile — heading text varies):
# "find ## Решения, ## Decisions, ## Принятые решения..."

# Role-based extraction (stable — roles map to multiple headings):
./tool get <id>#decisions       # → finds the decisions block regardless of heading language/format
./tool get <id>#actions,solution  # → extracts multiple blocks in one call
```

**Rules:**
- Roles are abstract labels: `decisions`, `actions`, `problem`, `solution`, `summary`, `context`.
- Each role maps to multiple heading patterns (aliases). The tool resolves them.
- Universal roles work across all artifact types. Type-specific roles exist for specialized content.
- Multi-block extraction (`#role1,role2`) reduces round-trips: one call for two blocks.

**Why it matters:** An agent that needs "decisions + actions" from a meeting transcript would otherwise: Read the file (500+ lines) → scan for headings → extract relevant sections. With block extraction: one call, two blocks, exact content. Token savings: 10-50×.

### 9. Interactive Decision Points

When the agent reaches a decision point (which item to work on, which approach to take, which artifact to review), it should present options via the platform's interactive UI rather than listing them as text and asking "which one?"

```
# Without interactive UI — text dump, operator types answer:
Agent: "Open topics: 1) Ask Evolution 2) Design System 3) Blocks 4) Gemini Proxy — which one?"
User: "2"

# With interactive UI — structured selection with preview:
Agent: [AskUserQuestion with 4 options, preview panel showing status + description]
User: [clicks/arrows → selects]
```

**Rules:**
- Use interactive selection at every **natural decision point**: topic selection, approach choice, triage, bet selection.
- Each option includes: short label (1-5 words) + description (what it means) + optional preview (context to decide).
- When >4 options exist, pre-filter first (by status, recency, relevance), then present top 4. The "Other" option catches the rest.
- `multiSelect: true` for non-exclusive choices: "Which gaps to fix?", "Which sections to include?"
- Preview panel for comparing: show metadata, status, key content for each option.
- Never use interactive UI for trivial confirmations ("proceed?" / "looks good?") — that's noise.

**Decision point taxonomy:**

| Decision type | Example | Options pattern |
|---|---|---|
| **Navigate** | Which topic to open? | Items from `./tool list` with status in preview |
| **Triage** | Where to file this input? | Zones/folders with description of what goes where |
| **Choose approach** | How to implement X? | 2-3 approaches with trade-offs in preview |
| **Prioritize** | Which bet/task next? | Ranked items with severity + cost in preview |
| **Multi-select** | Which lint issues to fix? | Checklist with file path + issue in description |

**Platform capabilities (Claude Code as of 2026-03):**
- 2-4 options per question, up to 4 questions per call
- Preview panel (markdown, monospace) for side-by-side comparison
- Single-select or multi-select mode
- Free-text "Other" always available as escape hatch
- Selection via arrow keys, numbers, or click

**Why it matters:** A text list requires the operator to read, parse, decide, and type an answer — 4 cognitive steps. An interactive picker reduces it to scan + select — 2 steps. For agents presenting 5+ decisions per session, this compounds into significant operator friction reduction.

**Use case catalog (abstract — adapt to your domain):**

| Use case | Type | When | Preview contains |
|---|---|---|---|
| **Session orientation** | Prioritize | Session start, after health check | Top tensions/gaps/stale with counts and severity |
| **Strategic choice** | Choose | Planning cycle, bet selection | Trade-offs: impact vs cost, meaning coverage |
| **Workflow navigation** | Navigate | "Return to topic X", "open deal Y" | Status, last update, key open question |
| **Content extraction** | Multi-select | After processing input (transcript, doc) | Available block types with counts and examples |
| **Input routing** | Triage | New file arrives for processing | Target zones with description of what fits |
| **Issue triage** | Multi-select | After validation/lint | Worst items with file, type, what's missing |
| **Approach selection** | Choose | Before implementation, multiple strategies | Each approach with effort, risk, trade-offs |
| **Message prioritization** | Prioritize | Multiple incoming messages/requests | Sender, subject, urgency, first lines |
| **Pattern matching** | Choose | Problem fits multiple known patterns | Problem/Solution summary per pattern |
| **Assembly/packaging** | Multi-select | Preparing deliverable from multiple sources | Artifact type, size, date, relevance |

**First-principles filter for when to use:**

All three must be true:
1. **Discrete options exist** — not open-ended input
2. **Agent can enrich** — preview adds context the operator doesn't have in their head
3. **Text exchange costs ≥2 messages** — listing + choosing → picker saves a round-trip

If any is false, use plain text. Pickers for binary yes/no or trivial choices are anti-elegant.

## Progressive Context Delivery

### The Problem with Front-Loading

Traditional approach: put all tool documentation in the system prompt (CLAUDE.md, etc.).

```markdown
## Tool X — 47 commands, 120 flags
| Command | Flags | Description |
... (500 tokens loaded on EVERY turn, whether needed or not)
```

Cost: 500 tokens × 200 turns = 100K tokens wasted per session on instructions the agent may never need.

### The Alternative: Tool-Embedded Hints

The tool itself teaches the agent what to do next, based on the data it just showed.

```
# System prompt (60 tokens):
Tool X: commands list, get, lint, scan. Use -j for JSON, -w for human.
Tool output includes contextual hints.

# Tool teaches the rest at point of use:
$ ./tool lint
tiers: t0:45 t1:30 t2:80 total:155 compliance:84%
0 errors, 15 warnings
hint: 16 below target. worst: insight(20%). try: ./tool lint --gap
```

The agent learns about `--gap` exactly when it needs it. Not 200 turns earlier.

### Hint Design Rules

**1. Conditional, not static.** A hint appears only when data suggests an action. Clean output = no hint. If compliance is 100%, no hint about gaps.

**2. Max 1 line.** Format: `hint: <reason>. try: <command>`. A hint longer than 1 line is a manual, not a hint.

**3. Concrete command, not advice.** Not "consider improving quality", but `try: ./tool lint --gap --type insight`.

**4. Chain of depth.** Each hint leads to a command whose output may contain its own hint, drilling deeper toward a specific action:

```
scan          → "11 gaps. try: ./tool lint --gap"
lint --gap    → "worst: digest(0%). try: ./tool lint --gap --type digest -w"
lint --gap --type digest -w  → "2 files. [add: source]"
```

Three hops from "what's wrong?" to "exactly what to do". The agent never needs to have memorized flag combinations.

**5. No hints in JSON.** API consumers parse structure, not prose. Hints are for agents and humans.

**6. Inline annotations for data.** Beyond footer hints, annotate data rows when something is notable:

```
type:guide status:active tier:0 (need 1, missing: id)
```

The parenthetical is an inline annotation — zero extra lines, appears exactly where the agent is looking.

### What Hints Replace

| Before (in system prompt) | After (in tool output) |
|---------------------------|------------------------|
| "After running lint, check compliance with --gap" | `hint: 16 below target. try: ./tool lint --gap` |
| "Insights need source: field for tier 2" | `[add: source]` next to each gap file |
| "Use --type to filter lint results" | `hint: worst: digest(0%). try: ./tool lint --gap --type digest -w` |
| "Run scan after editing files" | `hint: index stale (2h old). try: ./tool scan` |
| Table of all 7 commands and 15 flags | 60-token summary + hints teach flags on demand |

**Savings estimate:** 300-500 tokens removed from system prompt × every turn in the session.

### Anti-Patterns

| Anti-pattern | Why it fails |
|---|---|
| Hint on every run, even when clean | Noise fatigue — agent learns to ignore hints |
| Multi-line hints | Token cost approaches the documentation it replaced |
| Vague hints ("consider running lint") | Agent can't act without more context |
| Hints in JSON output | Breaks API parsers |
| Static hints unrelated to data | Not progressive — same as putting it in docs |

## Implementation Checklist

### Naming
- [ ] All command names are English words, 1 BPE token each
- [ ] Short aliases exist for frequent commands (first letter: `list`→`l`)
- [ ] All flag names have long (`--type`) and short (`-t`) forms
- [ ] Positional arguments resolve intelligently (known type name = type filter, unknown = text search)

### Output Tiers
- [ ] Default (no flag): minimal tokens, `id\tpath` format, footer count
- [ ] `-w`: aligned table with headers, full details, human-readable
- [ ] `-j`: compact JSON, stable schema, no hints
- [ ] `-q`: quiet — suppress hints and decorative footers
- [ ] All tiers from the same data path — no separate code branches for data retrieval

### Hints
- [ ] Every command can emit hints (except in `-j` and `-q` modes)
- [ ] Hints are conditional — triggered by data anomalies, not always present
- [ ] Hints contain executable commands (`try: ./tool <exact command>`)
- [ ] Hint chain exists: overview command → diagnostic → specific fix
- [ ] Inline annotations for per-record diagnostics (`tier:1 (need 2, missing: source)`)

### Views & Composites
- [ ] Named views for common diagnostic queries (`@health`, `@stale`)
- [ ] `@` with no name lists all available views
- [ ] At least one composite command for session-start health check
- [ ] Composite output is a digest, not concatenation of sub-commands

### Block Extraction
- [ ] Content accessible by semantic role (`#decisions`, `#actions`, `#solution`)
- [ ] Roles resolve to multiple heading patterns (aliases, languages)
- [ ] Multi-block extraction in one call (`#role1,role2`)
- [ ] Universal roles + type-specific roles

### Interactive Decision Points
- [ ] Decision points identified: navigate, triage, choose, prioritize, multi-select
- [ ] Options include label + description + preview where useful
- [ ] Pre-filter to ≤4 options when source list is longer
- [ ] multiSelect for non-exclusive choices
- [ ] Never used for trivial confirmations

### Speed
- [ ] Compiled binary or equivalent fast startup
- [ ] Index/cache for repeated queries
- [ ] Query commands < 50ms, full scan < 500ms

### Progressive Disclosure
- [ ] System prompt contains only: command list, mode flags, one-line purpose
- [ ] Flag combinations taught via hints at point of use
- [ ] Diagnostic details revealed through drill-down, not dumped upfront
- [ ] `-q` available as opt-out when agent has learned the tool

## When to Use

This pattern applies when building any CLI tool that an AI agent will use repeatedly:

- **File/artifact navigators** — index, search, filter, validate collections of files
- **Project status tools** — test runners, linters, build systems
- **Data query tools** — database CLIs, API wrappers, log analyzers
- **Workflow tools** — deployment, CI/CD, infrastructure management

The pattern does NOT apply to:
- One-shot tools (run once per session) — overhead of hints/tiers not justified
- Interactive tools (REPLs, editors) — different interaction model
- Tools with complex output that must be fully consumed (diff viewers, log streamers)

## Examples

### Example: Artifact Navigator (ask)

```bash
# Session start — one composite command:
./ask h
# 329 artifacts (disk), compliance:99%, 12 stale, 1 gap, 0 errors
# hint: 12 stale items. try: ./ask @stale

# Named view drills down:
./ask @stale
# insight-dtk-0115  knowledge/transcripts/insights/...  updated:01-15
# ...12 items

# Agent learns flags progressively via hints:
./ask lint --gap
# hint: worst: digest(0%). try: ./ask lint --gap -t digest -w

# Short aliases for experienced agent:
./ask l -t pattern -f id,status,title -S updated -l 5

# Semantic block extraction — no file reading:
./ask g insight-dtk-0302#decisions,actions
# ## Decisions ... ## Action Items ...

# Quiet mode — suppress hints when agent knows the tool:
./ask l -t pattern -c -q
# 16
```

Six commands demonstrate: composite → view → hint-guided learning → short aliases → block extraction → quiet mode. No documentation consulted.

### Example: Test Runner (hypothetical)

```bash
./test run
# 47 passed, 3 failed, 2 skipped  compliance:94%
# hint: 3 failures in auth/. try: ./test run --suite auth -v

./test run --suite auth -v
# FAIL auth/login_test.go:42  expected 200, got 401
# FAIL auth/token_test.go:18  token expired
# FAIL auth/refresh_test.go:7 missing env REFRESH_SECRET
# hint: 1 failure is env config. try: ./test env --check auth

./test env --check auth
# REFRESH_SECRET: missing (required by auth/refresh_test.go)
# hint: set REFRESH_SECRET in .env. try: echo "REFRESH_SECRET=test" >> .env
```

Same pattern: overview → focused diagnostic → specific fix instruction.

## Traps

- **Over-hinting** — if every command always emits a hint, the agent habituates and ignores them. Hints must be conditional.
- **Hint rot** — when command flags change, hints that reference old flags break the chain. Hints must be generated from current command structure, not hardcoded strings.
- **Premature optimization** — building three output tiers for a tool used twice per session is over-engineering. Start with agent-default only. Add `-w` and `-j` when real consumers need them.
- **Complex hint logic** — if generating the hint requires more computation than the command itself, the hint system is too complex. Hints should be simple conditionals on existing data.
- **Git-only indexing** — defaulting to git-tracked-only seems clean but loses valuable untracked content (gitignored personal data, local drafts, imported materials). Default to content-type filtering (extension whitelist) and offer git-only as opt-in.
- **View explosion** — named views are powerful but easy to over-create. Each view is a maintenance commitment. Start with 3-5 views for the most common diagnostics. Add more only when the agent or operator repeatedly composes the same query.
- **Alias collisions** — with single-letter aliases for both commands and flags, collisions are inevitable. Convention: uppercase flags for disambiguation (`-S` sort vs `-s` schema), commands use lowercase first-letter only. Document collisions explicitly.
