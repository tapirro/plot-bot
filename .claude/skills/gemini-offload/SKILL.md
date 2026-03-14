---
name: gemini-offload
description: Offload read-heavy, local, non-destructive investigation to Gemini CLI before spending large Claude context. Use this early in planning when the real need is repo summarization, candidate-file discovery, diff/log digestion, architecture mapping, pattern extraction, or option scouting across many files. Prefer it when you would otherwise read a lot just to narrow scope. Avoid it for final code edits, exact patch design, destructive actions, or tasks that depend heavily on nuanced conversation history.
allowed-tools: Bash(gemini *), Bash(timeout *), Bash(printf *), Bash(cat *), Bash(mktemp *), Bash(rm *), Bash(test *), Read, Grep, Glob
---

# Gemini Offload

Use Gemini CLI as a bounded local coprocessor for high-volume reading and compression.
Claude remains the primary engineer and final decision-maker.

If invoked with arguments, treat `$ARGUMENTS` as the offload brief.

## Success condition

Return a compact evidence pack that reduces Claude context load:

- a short answer to the question
- candidate files / directories to inspect next
- key evidence with file paths
- major uncertainties
- the single best next step for Claude

## Strong triggers

Use this skill when at least one is true:

- You would otherwise read many files just to find the right files.
- The task is mostly summarization, triage, mapping, clustering, or narrowing scope.
- You need a first-pass digest of logs, diffs, test failures, or stack traces.
- You want pattern extraction: "find similar implementations / tests / config shapes".
- You need a quick option scan before doing detailed reasoning.

Examples:

- Map the auth token refresh flow.
- Find where retries, backoff, or circuit breakers are implemented.
- Summarize a large diff into risks, touched subsystems, and follow-up files.
- Digest failing test output and identify the likely code path.
- Extract the best reference implementation for a new endpoint, widget, worker, or test.

## Do not use

Do not offload when the main work is:

- writing or editing the final patch
- making exact design judgments that depend on fine conversational nuance
- security-sensitive sign-off without local verification
- destructive or stateful actions
- anything involving commits, deploys, migrations, secret handling, or broad shell execution

## Default operating mode

- Keep the offload bounded, local, and read-only in spirit.
- Prefer giving Gemini file paths, folders, diffs, logs, or precise questions.
- Ask for compression, not essays.
- Never trust Gemini blindly; verify decisive claims in the repo before coding.
- After Gemini returns, continue with Claude's own reasoning and implementation.

## Offload protocol

1. State the narrow objective in one sentence.
2. Define scope tightly: files, folders, diff, log, subsystem, or question.
3. Include constraints:
   - use local repository context first
   - do not modify files
   - do not propose commits or shell actions
   - prefer concise evidence with file paths
4. Ask for the structured response format below.
5. Run Gemini in headless JSON mode.
6. Read the JSON result, extract the useful content, and discard verbosity.
7. Verify 2–3 critical claims locally before acting.

## Preferred output format to request from Gemini

Ask Gemini to return these sections inside its response:

1. `Summary` — 3–8 bullets
2. `Candidate paths` — most relevant files/directories, ordered
3. `Evidence` — short bullets with file paths and why they matter
4. `Unknowns / risks` — what may still be wrong or incomplete
5. `Next step for Claude` — one recommended action

## Canonical prompt template

Use this structure and fill it with the current task.

Goal:
<what Claude is trying to learn or narrow down>

Scope:
<files, folders, diff, logs, subsystem, or precise question>

Constraints:
- Use local repository context first.
- Treat this as read-only analysis.
- Be concise and evidence-oriented.
- Prefer file paths over long quotations.
- If evidence is weak, say so explicitly.

Return format:
- Summary
- Candidate paths
- Evidence
- Unknowns / risks
- Next step for Claude

## Model selection

Pick model by task weight. All models have 1M token context. Flat-rate on Google One AI Ultra — no per-token cost.

| Model | Speed | Reasoning | When to use |
|-------|-------|-----------|-------------|
| `gemini-2.5-flash-lite` | Fastest | Basic | Quick checks, classification, simple extraction, yes/no questions |
| `gemini-2.5-flash` | Fast | Good (thinking on/off) | Default for most offloads: file discovery, summarization, pattern scan |
| `gemini-3-flash-preview` | Fast | Strong (thinking levels) | Agentic workflows, multi-step analysis, code understanding |
| `gemini-2.5-pro` | Medium | Deep | Complex reasoning, cross-file architecture analysis, hard debugging |
| `gemini-3.1-pro-preview` | Slower | Strongest (3-tier thinking) | Hardest tasks: multi-system synthesis, ambiguous logic, novel patterns |

**Default:** omit `--model` — CLI auto-routes via flash-lite → flash/pro.
**Override:** `--model gemini-2.5-pro` when the task clearly needs deep reasoning.

**Heuristic:**
- Scanning / listing / extracting → flash-lite or flash (fast, cheap on quota)
- Understanding logic / architecture → flash-preview or pro
- Synthesizing across many files with nuance → pro or 3.1-pro-preview

## Parallelism and chaining

**Rate limit:** 60 requests/minute on Google One AI Ultra. Use this aggressively.

### Parallel dispatch

Launch multiple Gemini instances in parallel when subtasks are independent:

```bash
# Parallel — each runs in background, all finish faster than sequential
printf '<prompt_1>' | timeout 300 gemini --model gemini-2.5-flash --output-format json > /tmp/g1.json &
printf '<prompt_2>' | timeout 300 gemini --model gemini-2.5-flash --output-format json > /tmp/g2.json &
printf '<prompt_3>' | timeout 300 gemini --model gemini-2.5-pro --output-format json > /tmp/g3.json &
wait
```

When to parallelize:
- Scanning multiple directories/subsystems simultaneously
- Asking the same question from different angles
- Processing independent files or log segments
- Any task that decomposes into 2+ non-dependent parts

### Chaining (Gemini → Gemini)

Feed one Gemini's output as input to another for progressive compression or deeper analysis:

```bash
# Stage 1: Fast scan (flash) — broad, cheap
printf '<scan prompt>' | timeout 300 gemini --model gemini-2.5-flash --output-format json > /tmp/scan.json

# Stage 2: Deep analysis (pro) — reads stage 1 output + original files
cat /tmp/scan.json | python3 -c "import sys,json; print(json.load(sys.stdin)['response'])" > /tmp/scan_text.md
printf 'Based on this preliminary scan:\n---\n'"$(cat /tmp/scan_text.md)"'\n---\n<deep analysis prompt>' | \
  timeout 300 gemini --model gemini-2.5-pro --output-format json > /tmp/deep.json
```

Chaining patterns:
- **Fan-out → Fan-in:** N parallel flash agents scan → 1 pro agent synthesizes their outputs
- **Progressive compression:** Flash extracts raw data → Pro distills into insights → Claude gets only the insights
- **Validation chain:** Pro proposes answer → Flash checks claims against files → Claude gets verified result
- **Breadth → Depth:** Flash-lite identifies candidate files → Pro analyzes only the relevant ones

### When NOT to chain

- Simple single-file questions — one Gemini call is enough
- When Claude already has the context — don't re-process
- When the intermediate output is small enough for Claude to handle directly

## Command pattern

Prefer a bounded command like this, adapting quoting as needed:

`printf '%s\n' "<prompt>" | timeout 600 gemini --output-format json`

For specific model:

`printf '%s\n' "<prompt>" | timeout 600 gemini --model gemini-2.5-pro --output-format json`

If stdin handling is awkward, use:

`gemini --prompt "<prompt>" --output-format json`

## Compression rule

When you hand results back to Claude's main flow, compress aggressively:

- keep only the findings that change what Claude should read, edit, or test next
- prefer path lists and sharp evidence bullets
- omit generic explanation
- do not paste huge raw outputs unless the exact text matters

## Final responsibility

Gemini is for first-pass exploration and compression.
Claude owns the final reasoning, file reads, edits, tests, and decisions.
