---
id: pattern-phase-isolation
type: pattern
status: active
owner: vadim
updated: 2026-03-13
domain: plot-bot
tags: [architecture, subagents, token-economics]
confidence: observed
origin: ars-contexta-analysis + bridge-turn-research
basis: "AC: fresh context per phase. Our data: 27% budget = bridge turns at 104K avg context"
---

# Phase Isolation

## Problem

Multi-phase cognitive operations (analyze → generate → validate) executed in a single context accumulate intermediate results that degrade attention quality and waste budget on bridge turns (27% of total token spend).
There is no formal rule for when to isolate phases into separate subagent contexts.
By the third phase, the context window is polluted with raw data from phase one, causing attention degradation and redundant re-reads.

## Проблема

Операции с несколькими когнитивными фазами (analyze → generate → validate) выполняются в одном контексте. К третьей фазе контекст перегружен промежуточными результатами первых двух. Результат:

1. **Attention degradation** — модель теряет фокус на деталях при >80K context
2. **Bridge turns** — 27% бюджета = turn'ы-мосты между tool calls, каждый перечитывает весь контекст
3. **Context pollution** — артефакты фазы 1 (сырые данные) мешают фазе 3 (генерация)

## Solution

Use isolated subagent contexts (Agent tool) for each cognitive phase.
Each subagent receives a minimal input contract (specific files/data), a clear output path, and constraints preventing cross-phase pollution.
Phases exchange data through files, not conversation history.
This keeps each phase in the "accuracy zone" (<40K context) and eliminates bridge turns between phases.

## Решение

### Принцип

> Если операция имеет ≥2 фаз с разными когнитивными задачами, каждая фаза выполняется в изолированном контексте (Agent tool). Фазы обмениваются через файлы, не через conversation history.

### Когда изолировать

| Isolation Level | Критерий | Пример |
|----------------|----------|--------|
| **required** | Фазы параллелизуемы ИЛИ фаза читает >5 файлов | dispatch fix, meta-cycle domain scan |
| **recommended** | Фазы последовательны, но когнитивно разные | harvest→distill, research→verdict |
| **optional** | Фаза короткая (<2K output) | lint→fix single file |
| **skip** | Одна фаза или тривиальная операция | single file edit, quick query |

### Контракт субагента

Промпт каждого phase-субагента строится по шаблону:

```
## Task
{one_sentence_goal}

## Input
{list_of_files_or_data — ONLY what this phase needs}

## Output
Write result to: {exact_path}
Format: {format_spec}

## Constraints
- Read ONLY listed input files
- Write ONLY to output path
- {phase_specific_rules}
```

Генератор: `python3 tools/scripts/phase_prompt.py --task "..." --input "..." --output "..."`

### Операции с обязательной изоляцией

| Операция | Фазы | Тип субагента |
|----------|-------|---------------|
| Dispatch fix | batch N файлов | general-purpose × N (parallel) |
| Meta-cycle scan | per domain | Explore × N (parallel) |
| Harvest→Distill→Refine | 3 последовательных | general-purpose × 3 (sequential) |
| Transcript analysis | extract → structure → brief | general-purpose × 3 |
| Extraction pipeline | per batch | general-purpose × N |
| Rethink loop | gather → challenge → recommend | Explore + general-purpose + main |
| Deal analysis | research → analyze → verdict | Explore + general-purpose |

### Anti-patterns

1. **Over-isolation** — изолировать тривиальные операции (<2K output). Overhead субагента (~5K tokens) > экономия.
2. **Context starvation** — не дать субагенту достаточно контекста. Каждый субагент должен быть *самодостаточным*.
3. **Result echo** — пересказывать результат субагента в основном контексте. Файл записан — достаточно.

### Метрика

```bash
# Сравнить bridge_turns_pct до и после
python3 ~/.claude/plugins/cache/mantissa-claude-log/scripts/generate_reports.py --id <session>
# Target: bridge_turns_pct < 20% (baseline 27%)
```
