---
id: cross-agent-token-economics
title: Agent Token Economics
type: methodology
concern: [context-management, observability]
mechanism: [pipeline, scoring-model]
scope: system
lifecycle: [reflect, improve]
maturity: draft
domain: cross-agent/methodologies
confidence: observed
origin: harvest/vadim-assistant
basis: "empirical analysis of 112 sessions, 33K turns, 21K tool calls across 10 projects"
---

# Agent Token Economics — Research Report

**Статус:** исследование (не методология)
**Дата:** 2026-03-05
**Данные:** 112 сессий, 33,209 turns, 20,768 tool calls, 10 проектов, 546 MB JSONL
**Инструменты:** `context/sessions.db` (SQLite), `30_tools/scripts/session_etl.py`

---

## Problem

Claude Code агенты потребляют миллионы токенов контекста за сессию, но до сих пор не существовало эмпирического понимания структуры этих расходов. Тактические оптимизации отдельных tool-вызовов (например, «используй Grep вместо Bash grep») не дают измеримого эффекта из-за доминирования кэша (95% контекста). Без карты расходов невозможно определить, какие паттерны потребления являются неизбежными, а какие — устранимый waste, и где находятся стратегические рычаги с максимальным impact.

Масштаб проблемы: 112 сессий, 33K turns, 21K tool calls, 546 MB сырых логов — и при этом ни одной попытки систематического анализа. Существующие рекомендации (из документации Anthropic, community best practices) носят тактический характер — «используй Grep вместо cat», «читай частями» — и не учитывают, что 95.2% контекста составляют cache reads, где экономия на отдельных вызовах не даёт измеримого эффекта. Нужен мета-уровень анализа: не «как оптимизировать один вызов», а «какие структурные паттерны определяют 30-50% бюджета и как их изменить архитектурно».

## Solution

Эмпирический анализ 112 реальных сессий (33K turns, 21K tool calls, 546 MB JSONL) через трёхуровневую методологию: L1 — количественный token flow по SQL-запросам к нормализованной SQLite-базе; L2 — анализ tool sequences и переходных паттернов (bigrams, цепочки, кластеры); L3 — контент-анализ текста turn'ов на выборке 20 сессий. Пайплайн: сырые логи → ETL-нормализация → структурированная база → SQL + Python анализ → findings.

Ключевое отличие от ad-hoc оптимизаций — мета-уровень: вместо улучшения отдельных вызовов ищем структурные leverage points (bridge turns, orientation reads, session architecture), которые определяют 30-50% бюджета. Три уровня дают перекрёстную верификацию: L1 устанавливает количественные факты (bridge turns = 27.1% бюджета), L2 объясняет механизм (same-tool chains = 38% bridges, Read→Edit conversion = 39%), L3 верифицирует интерпретацию на контенте turn'ов (58% bridges — декларация намерения, 98.1% — обработка tool_result).

Результаты привязаны к конкретным архитектурным решениям: PTC (Programmatic Tool Calling) как устранение bridge turns, субагенты как context isolation для orientation reads, session checkpointing как контроль деградации длинных сессий (>200 turns = +27-39% cost per edit).

## 1. Что исследовали

**Вопрос:** Куда уходят токены при работе Claude Code агента? Какие паттерны потребления являются неизбежными, а какие — waste, который можно устранить? Какие стратегические рычаги дают максимальный выигрыш?

**Мотивация:** Оптимизация отдельных tool-вызовов (например, "используй Grep вместо Bash grep") экономит ~0 токенов благодаря кэшированию. Нужен мета-уровень — понять структуру расходов и найти leverage points.

## 2. Как исследовали

### Пайплайн

```
546 MB JSONL (session logs)
    ↓  session_etl.py (парсинг + нормализация)
12.8 MB SQLite (sessions / turns / tool_calls)
    ↓  SQL-запросы (Level 1-2: структурные паттерны)
    ↓  Python-скрипты на сырых JSONL (Level 3: контент turn'ов)
Findings
```

### Три уровня анализа

| Уровень | Что смотрим | Инструмент | Надёжность |
|---------|------------|------------|------------|
| **L1: Token flow** | Числа: input/output/cache по turn'ам | SQLite queries | Высокая — точные данные из API |
| **L2: Tool sequences** | Паттерны: какие инструменты за какими, цепочки, кластеры | SQLite + cross-turn analysis | Средняя — классификация эвристическая |
| **L3: Content analysis** | Текст turn'ов: что агент говорит, зачем читает файлы | Python на сырых JSONL | Средняя-низкая — на выборке 20 сессий |

### Ключевая формула

```
total_context = input_tokens + cache_read_input_tokens + cache_creation_input_tokens
```

Не путать: `input_tokens` ≠ total context. Это только fresh (non-cached) input. Cache reads (95.2%) доминируют. Total context — это то, что API "видит" и за что платится.

### Выборка

Полная выборка из `~/.claude/projects/` — все проекты, все сессии, за всё время. 10 проектов: voice-experiment (самый большой, 35 сессий), assistant (21), bot (14), 100ms (9), session (10), и 5 мелких.

---

## 3. Что нашли

### 3.1 Карта расходов (L1)

**Общий бюджет: 3,342M токенов контекста.**

| Категория | Turns | Context (M) | % бюджета | Описание |
|-----------|-------|-------------|-----------|----------|
| **Bridge turns** | 8,671 | **906** | **27.1%** | Между tool_result и следующим tool_use |
| Orientation (Read/Grep/Glob only) | 6,742 | 656 | 19.6% | Чтение для понимания, без action |
| Action turns (Edit/Write/Bash/Agent) | 11,232 | 1,159 | 34.7% | Продуктивная работа |
| Other reasoning | 3,520 | 353 | 10.6% | Текстовые ответы, планирование |
| Other mixed | ~3,044 | 268 | 8.0% | Task management, прочее |

**Главная находка: bridge turns — 27.1% бюджета.** Это turn'ы, в которых агент не вызывает инструментов и генерирует 1-5 output tokens. Они возникают между каждым tool_result и следующим tool_use. Каждый bridge загружает полный контекст (~104K avg).

### 3.2 Анатомия bridge turns (L2 + L3)

**Масштаб:** 8,671 turns × 104K = 906M tokens.

**Что внутри (content analysis, N=812 sampled):**
- 58% — декларация намерения: "Now let me edit X", "Let me read Y"
- 17% — narrative: описание следующего шага
- 7% — intent to run: "Test the imports"
- 18% — прочее (acknowledgments, plan steps, analysis)

**98.1% bridge turns — processing tool_result.** Агент получает результат инструмента, вербализирует следующий шаг (CoT), и в следующем turn'е выполняет его. Это НЕ wasted reasoning — это "клей" между tool calls, обязательный в текущей архитектуре.

**Кластеры по типу перехода:**

| Кластер | Bridges | Context (M) | Что происходит |
|---------|---------|-------------|----------------|
| same_tool_chain | 3,103 | 323 | Bash→br→Bash, Edit→br→Edit — повтор инструмента |
| task_management | 1,020 | 102 | TaskUpdate/Create bookkeeping |
| read_then_act | 681 | 73 | Read→br→Edit — "золотой путь" |
| verify_after_edit | 613 | 65 | Edit→br→Read — проверка результата |
| exploration_chain | 539 | 54 | Read/Grep→br→Read/Grep — ориентирование |
| test_after_edit | 489 | 52 | Edit→br→Bash — запуск тестов |
| investigate_after_bash | 453 | 50 | Bash→br→Read — расследование |
| fix_from_output | 294 | 33 | Bash→br→Edit — исправление по output |

**38% bridges — same-tool chains:** агент делает одно и то же несколько раз подряд, каждый раз с bridge между. Детализация:

| Chain | Bridges | Context (M) |
|-------|---------|-------------|
| Bash→br→Bash | 1,408 | 151 |
| Edit→br→Edit | 958 | 95 |
| Read→br→Read | 454 | 46 |
| Grep→br→Grep | 190 | 19 |
| Write→br→Write | 93 | 10 |

### 3.3 Read necessity (L3)

**670 Read'ов проанализировано:**
- 39% → Edit в течение 4 turns (необходимые — pre-edit read)
- **61% → НЕ приводят к Edit** (orientation / reference)

**По типу файла:**

| Тип | Reads | Useful% | Wasted ctx (M) |
|-----|-------|---------|----------------|
| source_code | 326 | 47.5% | 15.4 |
| documentation | 114 | **20.2%** | 8.6 |
| templates | 140 | 50.7% | 6.2 |
| config_data | 32 | **15.6%** | 2.4 |
| data_files | 9 | **0.0%** | 0.9 |

Documentation reads — 80% wasted. Data files — 100% wasted.

**Якорные файлы (re-read ≥5 сессий):**
- `pipeline.py` — 759 reads / 24 sessions (31.6 reads/session!)
- `voice.html` — 496 reads / 22 sessions
- `app.py` — 419 reads / 28 sessions

### 3.4 Стратегические паттерны (L1 + L2)

**Session length × efficiency:**

| Размер сессии | Tok/edit | vs baseline |
|--------------|----------|-------------|
| <50 turns | 520K | baseline |
| 50–200 | 545K | +5% |
| 200–500 | 659K | **+27%** |
| >1K turns | 722K | **+39%** |

Длинные сессии (>200 turns) стоят 27-39% дороже за единицу продуктивной работы.

**Фазы сессии:**

| Фаза | Avg context | Edit % | Explore % |
|------|-------------|--------|-----------|
| 0-19 turns | 47K | 6.1% | **31.0%** |
| 20-49 | 65K | **20.7%** | 15.3% |
| 50-99 | 90K | 19.3% | 14.1% |
| 200-499 | 112K | 15.2% | 20.0% |
| 500+ | 109K | 14.7% | **23.7%** |

Пик продуктивности: turns 20-50. После turn 200 exploration ползёт обратно вверх — агент "забывает" и re-explores.

**Boot tax:** в среднем 25 turns до первого Edit. 4.2% бюджета (139M) на "разогрев".

**Error cascades:** до 69% ошибок идут подряд (кластеризуются). 824 error turns × 97K = 80M wasted.

**Bash ∝ 1/productivity:** sessions с >50% Bash дают edit rate 0.134 vs 0.280 у structured (<10% Bash).

**Planning + Tasks:** sessions с plan mode / task structure дают +16-28% edit rate (корреляция, не causation).

---

## 4. Подтверждения и перекрёстные проверки

### Что подтверждено из нескольких источников

| Находка | Источники | Уверенность |
|---------|-----------|-------------|
| Bridge turns = 27% бюджета | L1 (SQL), L3 (content parsing) | **Высокая** — числа сходятся |
| 98% bridges = tool_result processing | L3 (content), подтверждено prev/next turn analysis | **Высокая** |
| 61% reads не ведут к edit | L3 (670 reads), cross-checked с L2 (file access patterns) | **Средняя** — выборка 20 сессий |
| Long sessions дороже | L1 (token/edit по размерам), L2 (exploration creep at 200+) | **Высокая** — два независимых сигнала |
| Same-tool chains = 38% bridges | L2 (SQL bigrams), L3 (content clusters) | **Высокая** |

### Что требует осторожности

| Находка | Ограничение |
|---------|-------------|
| Planning +16-28% edit rate | Корреляция. Сложные задачи и так требуют плана и больше edits |
| Bash ∝ 1/productivity | Возможно инвертирована причинность: сложные/неструктурированные задачи требуют Bash |
| Boot tax 25 turns | Средняя по всем проектам. Разброс: от 6 (fast start) до 79 (slow start) |
| Read necessity 39%/61% | Выборка 20 сессий, не все проекты. voice-exp перепредставлен |

### Чего мы НЕ знаем

- **Качество bridge reasoning.** Bridge turns содержат CoT, который может улучшать качество следующего tool call. Убрав bridges, мы можем ухудшить decision quality.
- **Cache economics.** 95% context = cache reads. Реальная стоимость bridge ≠ 104K "свежих" токенов. Нужен cost model с учётом цен на cache vs fresh.
- **Subagent overhead.** Субагент тоже имеет boot cost. Если задача маленькая, overhead субагента может превысить bridge cost.
- **Content of "useful" reads.** Мы знаем что 61% reads не ведут к edit, но не знаем — они влияют на quality решений?

---

## 5. Ключевая находка: PTC как архитектурное решение

### Что такое PTC

**Programmatic Tool Calling** — фича Claude API (beta). Модель пишет Python-скрипт, вызывающий инструменты в sandbox. Промежуточные результаты НЕ попадают в контекст — только stdout скрипта.

Reference: `/Users/polansk/Developer/mantissa/artifacts/ptc-cheatsheet.md`

### Как PTC решает проблему bridge turns

Текущий паттерн (Claude Code):
```
Read(A) → [bridge 104K] → Read(B) → [bridge 104K] → Read(C) → [bridge 104K] → Edit(A)
= 4 inference passes, 3 bridges, ~312K bridge context
```

PTC-паттерн:
```python
# 1 inference pass, 0 bridges, 0 intermediate context
a = await Read("A")
b = await Read("B")
c = await Read("C")
# agent sees only stdout:
print(f"Files read. Key function in A at line 42. B imports X. C has bug at line 100.")
await Edit("A", fix_based_on_analysis)
```

### Маппинг наших данных → PTC savings

| Наш паттерн | Bridges | Ctx (M) | PTC-решение |
|---|---|---|---|
| Bash→br→Bash (retry) | 1,408 | 151 | `for cmd in variants: r = await Bash(cmd); if ok: break` |
| Edit→br→Edit (multi-file) | 958 | 95 | `for f in files: await Edit(f, patch[f])` |
| Read→br→Read (explore) | 454 | 46 | `data = {f: await Read(f) for f}; print(summary)` |
| Read→br→Edit (orient→act) | 681 | 73 | `content = await Read(f); await Edit(f, fix(content))` |
| Edit→br→Bash (edit→test) | 489 | 52 | `await Edit(f, ...); print(await Bash("pytest"))` |

**Оценка savings:** ~3,990 collapsible bridges × 104K ≈ **415M tokens (12.4% бюджета)**.

### Проблема: PTC НЕ работает в Claude Code

Claude Code не передаёт `allowed_callers` в API. Open issue: [#12836](https://github.com/anthropics/claude-code/issues/12836), с декабря 2025, не реализован.

### Workarounds доступные сейчас

| Уровень | Механизм | Что покрывает | Overhead |
|---------|----------|--------------|----------|
| **1. Bash-скрипты** | inline python/bash вместо цепочки Bash→Bash | Bash chains (151M) | Минимальный |
| **2. Субагенты** | Agent("read A,B,C, summarize X") | Read chains (46M), exploration (54M) | Boot cost субагента |
| **3. MCP-сервер + API PTC** | MCP proxy → Claude API с PTC | Всё (415M) | Архитектурная сложность |

---

## 6. Выводы

### Три рычага по убыванию impact

**Рычаг 1: Устранение bridge turns (~415M, 12% бюджета)**
Bridges — самая дорогая waste-категория. Каждый bridge = ~104K context для 1-5 tokens output. PTC устраняет их архитектурно, но не доступен в Claude Code. Workaround: Bash-скрипты для цепочек + субагенты для Read bursts.

**Рычаг 2: Context isolation через субагенты (~500M, 15% бюджета)**
6,742 orientation turns можно делегировать субагентам с 20K context вместо 100K. 61% Read'ов — reference/orientation. Read bursts (3-14 файлов подряд) — идеальные субагентные задачи. Субагент = "бедный PTC" — изоляция контекста + compression.

**Рычаг 3: Session architecture (~200M, 6% бюджета)**
Оптимальная длина сессии: 100-200 turns. >200 turns = +27-39% cost per edit. Boot tax 25 turns сжимается до 5 через parallel subagent warmup. Error circuit breaker после 3 consecutive errors.

### Что НЕ является рычагом

- **Bash vs Read** (0 savings — cache всё нивелирует)
- **Тактические оптимизации отдельных tool calls** (< 1% impact)
- **Нарезка файлов** (работает для одного проекта, не системное решение)

---

## 7. Рекомендации

### Действия сейчас (без изменения инфраструктуры)

1. **"Bash batch" паттерн.** Когда видишь потенциальную цепочку Bash→Bash→Bash — напиши inline python-скрипт. Один Bash(script) вместо 3 Bash + 2 bridges.

2. **"Subagent for Read bursts".** Когда нужно прочитать 3+ файлов для ориентации — Agent("read files X, Y, Z; answer question Q"). Не читать в основном контексте.

3. **"Subagent for debugging".** Bash→br→Bash retry loops = Agent("make this command work, return result"). Изолирует debugging от основного контекста.

4. **Session checkpointing.** При приближении к 200 turns — checkpoint state, рассмотреть fresh session.

### Требует исследования

5. **MCP-proxy PTC.** Создать MCP-сервер, вызывающий Claude API с PTC. Claude Code использует его как один tool. Нужна оценка: overhead vs savings.

6. **Мониторинг PTC в Claude Code.** Issue #12836. Когда появится — мгновенный switch, паттерны уже известны.

### Требует методологизации

7. **Subagent delegation decision tree.** Формализовать: когда делегировать, с каким prompt, как возвращать результат. Сейчас субагенты используются ad-hoc (240 calls, 0 параллельных).

---

## 8. Открытые вопросы и следующие шаги

### Гипотезы для проверки

**H-F: Bridge quality impact.** Убрав bridges, ухудшится ли quality решений? Bridge содержит CoT. Эксперимент: сравнить sessions с длинными vs короткими bridges — одинаковый ли error rate?

**H-G: Subagent ROI threshold.** При какой минимальной задаче субагент окупает свой boot cost? Если boot = 5K tokens, а bridge = 104K, порог = задача из 1 bridge. Но субагент может ошибиться и стоить дороже. Нужен эмпирический тест.

**H-H: Cache cost model.** 95% context = cache reads. Cache reads стоят 10% от fresh input (Anthropic pricing). Реальная цена bridge = 104K × 0.1 = 10.4K "эквивалентных" токенов? Меняет ли это приоритеты?

**H-I: Parallel subagent scaling.** Сейчас 0 параллельных Agent calls. Если boot orientation (25 turns) заменить на 3 parallel subagents — каков реальный speedup и context savings?

**H-J: PTC proxy feasibility.** MCP-сервер + Claude API с PTC: latency, cost, reliability. Стоит ли overhead?

### Данные для следующего раунда

- **Cost model:** нужны реальные цены (fresh vs cache) для перевода token savings в доллары
- **Subagent sessions:** нужно больше сессий с активным использованием субагентов для сравнения
- **A/B baseline:** провести 2-3 сессии "как обычно" и 2-3 "с conscious subagent delegation" на сравнимых задачах
- **PTC experiment:** использовать Claude API напрямую (не через Claude Code) с PTC для типичной задачи, замерить реальную разницу

### Инфраструктура для continuous monitoring

| Метрика | Целевое | Текущее | Как мерить |
|---------|---------|---------|------------|
| Bridge % от бюджета | <15% | 27.1% | session_etl + SQL |
| Read→Edit conversion | >50% | 39% | L3 analysis |
| Avg decision gap | <5 turns | 4.3-7.3 | SQL bigrams |
| Session length | 100-200 | varies | turns table |
| Subagent usage | growing | 240 total, 0 parallel | tool_calls |
| Boot time | <10 turns | 25 avg | first edit turn |

---

## Приложения

### A. Инструменты

| Файл | Назначение |
|------|-----------|
| `30_tools/scripts/session_etl.py` | JSONL → SQLite ETL (incremental) |
| `context/sessions.db` | SQLite database с sessions/turns/tool_calls |
| `/Users/polansk/Developer/mantissa/artifacts/ptc-cheatsheet.md` | PTC reference |

### B. Ключевые SQL-запросы

**Bridge turns:**
```sql
SELECT COUNT(*), CAST(SUM(total_context)/1e6 AS INT) as ctx_M
FROM turns t
WHERE output_tokens <= 5
  AND NOT EXISTS (SELECT 1 FROM tool_calls tc WHERE tc.turn_id = t.id);
-- → 8,671 turns, 906M tokens
```

**Bridge transitions:**
```sql
-- prev_tool → bridge → next_tool (see full query in research notes)
-- Top: Bash→br→Bash (1,408), Edit→br→Edit (958), Read→br→Read (454)
```

**Read necessity:**
```sql
-- L3 Python analysis: 670 reads, 39% followed by Edit within 4 turns
```

**Session length efficiency:**
```sql
-- Tok/edit by session size buckets
-- <50: 520K, 50-200: 545K, 200-500: 659K, >1K: 722K
```

### C. Limitations

- Данные только из Claude Code (не из API-based agents)
- voice-experiment перепредставлен (5,555 turns в одной сессии)
- Cache read dominance (95.2%) означает что token counts ≠ dollar costs
- Content analysis (L3) на выборке 20 сессий — может быть bias
- Нет baseline: мы не знаем "нормальный" bridge % для Claude Code

### D. Версионирование

| Дата | Что изменилось |
|------|---------------|
| 2026-03-05 | Первый research report. L1-L3 analysis, PTC mapping |
