---
id: cross-artifact-centric-interface
title: Artifact-Centric Agent-Human Interface
type: pattern
concern: [artifact-management]
mechanism: [pipeline, registry]
scope: system
lifecycle: [act, reflect]
origin: harvest/voic-experiment
origin_findings: [17, 20, 29, 30]
maturity: draft
domain: cross-agent/patterns
confidence: observed
basis: "distilled from voic-experiment harvest, 65 findings from voice agent sessions"
---

# Artifact-Centric Agent-Human Interface

<!-- CORE: load always -->
## Problem

Agents and humans must see the same working state, but traditional architectures create divergent views. When a database is the shared state store, the agent interacts through queries, the human interacts through a dashboard, and debugging requires SQL or API introspection to understand what actually happened. The data is hidden behind layers of abstraction.

This architecture creates four concrete problems. First, hidden state: the agent's working memory and the database hold different views of reality, with no simple way to verify consistency. Second, duplicated logic: business rules exist in both the agent's instructions and the API handlers, inevitably diverging. Third, debugging friction: understanding why the agent made a decision requires reconstructing state from database snapshots, API logs, and agent transcripts -- three separate systems. Fourth, infrastructure dependency: the database and API server become single points of failure independent of the agent's actual work, which is fundamentally about reading inputs and producing outputs.

## Solution

A **4-layer architecture** with files on disk as the single source of truth, where each layer adds convenience but no layer adds critical functionality:

1. **Layer 1 (Data):** All working state exists as files -- JSON for machine state, markdown with YAML frontmatter for human-reviewed artifacts, JSONL for append-only event logs. Git provides versioning, diffing, and audit trail for free. If the agent crashes, `cat state.json` shows the complete picture.

2. **Layer 2 (CLI Tools):** All business logic lives in standalone CLI scripts that read files, compute, and write files. Each tool is independently testable, composable via pipes, and callable by both agent and human. An orchestrator script composes tools into named pipelines.

3. **Layer 3 (Thin API):** A minimal read-only API server that serves file contents as JSON over HTTP. Zero business logic -- it is a file reader with an HTTP interface. The system works fully without it.

4. **Layer 4 (Dashboard):** A client-side visualization that fetches from the API and renders charts, tables, and status views. Read-only, optional, and refresh-tolerant since all state is in files.

The system degrades gracefully: with all four layers, the human sees a dashboard while the agent operates via CLI. With only layers 1 and 2, the human inspects files with `cat` and `git diff` while the agent operates normally. With layer 1 alone, the files on disk remain inspectable, diffable, and recoverable.

## Implementation

### Structure

```
{WORKSPACE}/
├── {STATE_DIR}/
│   ├── {PRIMARY_STATE_FILE}        # current state (JSON, overwritten)
│   ├── {HISTORY_FILE}              # append-only event log (JSONL)
│   └── {SECONDARY_STATE}/          # per-entity state files
│       ├── {ENTITY_1}.json
│       └── {ENTITY_2}.json
├── {ARTIFACTS_DIR}/
│   ├── {ARTIFACT_TYPE_A}/          # structured artifacts (markdown + frontmatter)
│   │   ├── {ARTIFACT_1}.md
│   │   └── {ARTIFACT_2}.md
│   └── {ARTIFACT_TYPE_B}/          # data artifacts (JSON/JSONL)
│       ├── {ARTIFACT_3}.json
│       └── {ARTIFACT_4}.jsonl
├── {RENDERED_DIR}/                  # auto-generated human-readable views
│   ├── {RENDERED_VIEW_1}.md         # generated from state, not edited manually
│   └── {RENDERED_VIEW_2}.md
├── {TOOLS_DIR}/
│   ├── {TOOL_1}.py                  # CLI tool: reads/writes state files
│   ├── {TOOL_2}.py                  # CLI tool: reads/writes artifact files
│   └── {ORCHESTRATOR}.py            # composes tools into pipelines
├── {API_DIR}/
│   └── {API_SERVER}.py              # thin read-only API over state files
└── {DASHBOARD_DIR}/
    └── index.html                   # client-side visualization
```

### Layer 1: Data as Markdown + Structured State

All working artifacts exist as files. Two formats, chosen by access pattern:

| Format | When to Use | Properties |
|--------|-------------|------------|
| Markdown + YAML frontmatter | Human-authored or human-reviewed artifacts (plans, analyses, reports) | Readable in any editor, searchable with grep, diffable in git |
| JSON | Current state, configuration, single-entity records | Machine-parseable, schema-validatable, overwrite-on-update |
| JSONL | Event logs, history, append-only records | Append-only, one record per line, streamable, no corruption on crash |

Key principle: **every piece of state is a file.** No in-memory-only state, no database-only state. If the agent crashes, the state on disk is the complete picture.

Git provides for free:
- **Versioning:** `git log {file}` shows full history
- **Diffing:** `git diff` shows what changed between any two points
- **Audit trail:** Who changed what, when, with what commit message
- **Branching:** Experimental changes without risking main state

### Layer 2: CLI Tools (Agent's Primary Interface)

All business logic lives in CLI tools — standalone scripts that read files, compute, write files:

```
{TOOL}
├── Input:   file path(s) or stdin
├── Logic:   pure computation (no network, no database)
├── Output:  file path(s) or stdout
└── Exit:    0 = success, non-zero = failure with message on stderr
```

Design principles:
- **Standalone:** Each tool works independently, has its own `--help`
- **Composable:** Output of one tool feeds input of the next via files or pipes
- **Testable:** Pure functions that transform files — easy to unit test
- **Idempotent:** Running twice with same input produces same output
- **Human-callable:** A human can run the same tools from terminal

Pipeline composition example:

```bash
# Agent runs this pipeline:
{TOOL_1} --input {STATE_DIR}/{PRIMARY_STATE_FILE} --output /tmp/intermediate.json
{TOOL_2} --input /tmp/intermediate.json --output {ARTIFACTS_DIR}/{ARTIFACT_TYPE_B}/{RESULT}.json
{TOOL_3} --input {ARTIFACTS_DIR}/{ARTIFACT_TYPE_B}/{RESULT}.json --render {RENDERED_DIR}/{RENDERED_VIEW_1}.md

# Human can run the exact same commands to reproduce or debug
```

The {ORCHESTRATOR} composes tools into named pipelines:

```bash
{ORCHESTRATOR} run {PIPELINE_NAME}
# Executes: {TOOL_1} → {TOOL_2} → {TOOL_3} in sequence
# Stops on first non-zero exit code
```

### Layer 3: Thin API (Bridge)

A minimal API server that reads files and returns JSON. Zero business logic:

```
{API_SERVER}
├── GET /state              → reads {PRIMARY_STATE_FILE}, returns JSON
├── GET /artifacts/{type}   → reads {ARTIFACTS_DIR}/{type}/, returns list
├── GET /artifacts/{type}/{id} → reads single artifact, returns JSON
├── GET /history            → reads {HISTORY_FILE}, returns JSONL
├── GET /rendered/{view}    → reads {RENDERED_DIR}/{view}, returns markdown
└── GET /health             → checks file system access, returns status
```

Rules:
- **Read-only:** API never writes files. All mutations go through CLI tools
- **No business logic:** API is a file reader with HTTP interface
- **Optional:** System works fully without API (CLI-first)
- **Stateless:** No sessions, no in-memory cache of file contents (reads fresh on every request)
- **File-watching (optional):** Notify dashboard of changes via SSE or WebSocket

### Layer 4: Dashboard (Human's Visual Interface)

A single-page application that fetches from the API and renders visualizations:

```
{DASHBOARD}
├── KPI cards              ← from GET /state
├── State flow diagrams    ← from GET /state (rendered client-side)
├── Distribution charts    ← from GET /artifacts/{type} (aggregated client-side)
├── Interactive tables     ← from GET /artifacts/{type} (sortable, filterable)
├── History timeline       ← from GET /history
└── Rendered views         ← from GET /rendered/{view} (markdown → HTML)
```

Rules:
- **Read-only:** Dashboard never mutates state. All changes go through CLI tools (via agent or human)
- **No backend rendering:** Pure client-side JavaScript
- **Optional:** System works fully without dashboard (CLI + files are the primary interface)
- **Refresh-tolerant:** Dashboard can be closed and reopened without losing state (state is in files)

### Graceful Degradation

| Available Layers | Capability |
|-----------------|------------|
| 1 + 2 + 3 + 4 | Full experience: agent operates via CLI, human sees dashboard |
| 1 + 2 + 3 | Agent operates via CLI, human queries API with curl |
| 1 + 2 | Agent operates via CLI, human inspects files with cat/git |
| 1 only | Files on disk — inspectable, diffable, recoverable |

Each layer adds convenience but no layer adds critical functionality. The system is correct at Layer 1.

### Decision Rules

| Situation | Action |
|-----------|--------|
| New state needed | Add a JSON file to {STATE_DIR}, not a database table |
| New business logic needed | Add a CLI tool to {TOOLS_DIR}, not an API endpoint |
| New visualization needed | Add a dashboard component that reads from existing API |
| API endpoint needs computation | Move computation to CLI tool, API reads the result file |
| Human wants to debug agent behavior | `cat {STATE_DIR}/{PRIMARY_STATE_FILE}` + `git log` |
| Agent needs to inspect its own state | Read the same files via CLI tool |
| Database suggested for performance | Evaluate if JSONL + file indexing solves it first |
| Real-time updates needed | Add file-watching + SSE to Layer 3, do not move state to database |

### Configuration

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `{WORKSPACE}` | Root directory for all artifacts | "project/" | yes |
| `{STATE_DIR}` | Directory for state files | "state/" | yes |
| `{PRIMARY_STATE_FILE}` | Main state file (JSON) | "state.json" | yes |
| `{HISTORY_FILE}` | Append-only event log (JSONL) | "history.jsonl" | yes |
| `{ARTIFACTS_DIR}` | Directory for working artifacts | "artifacts/" | yes |
| `{RENDERED_DIR}` | Directory for auto-generated views | "rendered/" | no |
| `{TOOLS_DIR}` | Directory for CLI tools | "tools/" | yes |
| `{ORCHESTRATOR}` | Pipeline composition script | "orchestrate.py" | yes |
| `{API_SERVER}` | Thin API server script | "api.py" | no |
| `{DASHBOARD_DIR}` | Dashboard static files | "dashboard/" | no |
| `{API_PORT}` | Port for API server | 8080 | no |

<!-- REFERENCE: load on adoption -->
## Adaptation Guide

### Prerequisites
- A file system accessible to both agent and human operator
- Git (or equivalent VCS) for versioning and audit trail
- CLI/scripting capability (Python, Bash, or equivalent)
- A domain where working state can be serialized to JSON/markdown (most domains qualify)

### Steps to Adopt
1. Define your state model — what is the primary state object? What entities need per-entity files?
2. Create {STATE_DIR} with {PRIMARY_STATE_FILE} (JSON) and {HISTORY_FILE} (JSONL)
3. Move existing business logic into CLI tools in {TOOLS_DIR} — each tool reads files, computes, writes files
4. Create the {ORCHESTRATOR} to compose tools into named pipelines
5. Verify the agent works with Layers 1 + 2 only (no API, no dashboard)
6. Add the thin API (Layer 3) — read-only endpoints that serve file contents as JSON
7. Add the dashboard (Layer 4) — client-side rendering from API responses
8. Add rendered views in {RENDERED_DIR} for human-readable summaries auto-generated from state
9. Set up git hooks or CI to validate file schema on commit
10. Document the file schema so both agent and human know the contract

### What to Customize
- File formats per artifact type (markdown for human-reviewed, JSON for machine-processed, JSONL for logs)
- State model structure (depends entirely on your domain)
- CLI tool granularity (one tool per operation vs. one tool with subcommands)
- Dashboard visualizations (depends on what the human operator needs to see)
- API endpoint structure (match your file directory layout)
- Rendered view templates (what summaries are most useful?)

### What NOT to Change
- Files as single source of truth — introducing a database as primary store breaks the inspectability guarantee
- CLI-first business logic — putting logic in the API makes the system dependent on the API server
- Read-only API — allowing API writes creates two mutation paths, leading to inconsistency
- Read-only dashboard — allowing dashboard mutations bypasses CLI tool validations
- Layer independence — each layer must work without the layers above it
- Git for versioning — replacing git with custom versioning loses the ecosystem (diff, log, blame, branch)
- JSON/JSONL for structured data — custom binary formats break inspectability with standard tools

<!-- HISTORY: load for audit -->
## Origin
- **Source agent:** voic-experiment
- **Findings:** [17] CLI tools as primary agent interface with pipeline composition, [20] Canonical data models as files shared across all tools, [29] Thin read-only API as bridge between file state and dashboard, [30] Client-side dashboard rendering from API without backend logic
- **Discovered through:** Building an agent that needed both autonomous operation and human oversight. Initial architecture used a database as the shared state — the human needed a dashboard, the agent needed queries, and debugging required SQL. Switching to files-on-disk as the source of truth made the entire system inspectable with `cat` and `git diff`. Business logic moved from API handlers to CLI tools, making it testable without spinning up a server. The API shrank to a file reader. The dashboard became optional. The key insight: the agent and the human should see the same data through different lenses (CLI vs. browser), not through different data stores.
- **Evidence:** Debugging time reduced dramatically — `cat state.json` replaced "check the database, check the API response, check the dashboard." CLI tools were testable with simple file fixtures. The system survived API server outages without data loss (files were intact). Git history provided a complete audit trail for free.

## Related Patterns
- [Autonomous Loop Protocol](../patterns/loop_protocol.md) — loop state persistence (state.json + JSONL) follows this pattern's Layer 1
- [Validation Gates](../patterns/validation_gates.md) — gates can validate file schema before commits
- [Scenario-First Testing](../methodologies/scenario_first_testing.md) — test results stored as structured files follow this pattern
- [Monitoring Principles](../best_practices/monitoring_principles.md) — dashboard (Layer 4) is a monitoring interface over file-based state
- [Observability Engine](../patterns/observability_engine.md) — trace data stored as JSONL files follows Layer 1; dashboard visualizes trace data
