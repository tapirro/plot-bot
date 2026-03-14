---
id: pattern-deliberation
type: pattern
status: draft
owner: vadim
created: 2026-03-07
domain: cross-agent/methodologies
origin: self
confidence: validated
basis: "used in 10+ topic discussions with structured lifecycle"
---

# Structured Deliberation Pattern

Паттерн для ведения дискуссий с фиксацией этапов обсуждения и решений. Обеспечивает трассируемость: от вопроса → через аргументы → к решению → в базу знаний.

## problem: Problem

Стратегические и архитектурные решения принимаются в чате и теряются между сессиями.
Нет трассируемости: невозможно восстановить, почему было выбрано решение A, а не B.
Аргументы, альтернативы и контраргументы не фиксируются — при повторном обсуждении
всё начинается с нуля. Решения не мигрируют в базу знаний и остаются в контексте
одной сессии, недоступные для будущих.

## solution: Solution

Формализованный lifecycle обсуждения: OPEN → EXPLORE → CONVERGE → DECIDED → MIGRATED.
Каждая тема получает папку в `work/topics/` с двумя ключевыми файлами:
- `TOPIC.md` — карточка темы с альтернативами, trade-off матрицей и финальным решением
- `positions.md` — append-only лог раундов обсуждения с аргументами и evidence

Триггеры на ключевое слово «тема» запускают соответствующую фазу автоматически.
После принятия решения артефакт мигрирует в knowledge base (`knowledge/` или `knowledge/playbook/`),
обеспечивая долгосрочную доступность. Git обеспечивает audit trail,
а индекс `work/topics/INDEX.md` — быстрый поиск по всем обсуждениям.

## Когда использовать

- Вадим кидает тему/вопрос, требующий обсуждения с разных сторон
- Есть несколько альтернатив и нужно выбрать
- Решение повлияет на архитектуру, методологию или бизнес-процесс
- Нужно зафиксировать "почему так решили" для будущих сессий

**НЕ использовать** для: простых задач ("сделай X"), документирования готовых решений, ad-hoc вопросов без долгосрочных последствий.

## Триггеры

Ключевое слово: **«тема»**. Четыре команды:

| Фраза Вадима | Действие | Фаза |
|-------------|----------|------|
| **«есть тема...»** | OPEN — создать deliberation, начать обсуждение | → OPEN |
| **«вернёмся к теме...»** | RESUME — найти deliberation по ключевым словам, загрузить контекст, продолжить | (текущая фаза) |
| **«тему фиксируем»** | CHECKPOINT — сохранить промежуточное состояние в файлы | (текущая фаза) |
| **«тему закрываем»** | CLOSE — подвести итоги, зафиксировать решение, мигрировать артефакт | → DECIDED → MIGRATED |

### Протокол команд

**«есть тема...»** — после этих слов идёт формулировка вопроса/проблемы.
1. Извлечь вопрос из реплики
2. Создать `work/topics/YYYY-MM-DD_slug/`
3. Заполнить TOPIC.md (question, context, stakes)
4. Предложить альтернативы, спросить что упущено
5. Перейти к EXPLORE

**«вернёмся к теме...»** — после этих слов идёт название/ключевые слова темы.
1. Искать в `work/topics/` по: slug в имени папки, question в TOPIC.md frontmatter
2. Если найдено несколько — показать список, спросить какую
3. Прочитать TOPIC.md и positions.md — восстановить контекст
4. Показать Вадиму краткий статус: фаза, альтернативы, последний раунд
5. Продолжить с текущей фазы

**«тему фиксируем»** — промежуточное сохранение без закрытия.
1. Обновить TOPIC.md: текущие альтернативы, trade-offs (UPSERT)
2. Добавить новый раунд в positions.md с итогами обсуждения (APPEND)
3. Подтвердить Вадиму что зафиксировано
4. Тема остаётся в текущей фазе

**«тему закрываем»** — финализация.
1. Спросить Вадима: какое решение? (если не очевидно из контекста)
2. Заполнить Decision секцию в TOPIC.md
3. Определить целевой файл для миграции
4. Создать/обновить артефакт в knowledge base
5. Заполнить Migration секцию, поставить `status: MIGRATED`
6. Обновить `work/topics/INDEX.md`

## Жизненный цикл темы

```
OPEN → EXPLORE → CONVERGE → DECIDED → MIGRATED
```

| Фаза | Что происходит | Артефакт |
|------|---------------|----------|
| OPEN | Фиксация вопроса, контекста, stakes | Topic card |
| EXPLORE | Сбор аргументов, позиций, evidence | Positions log (append-only) |
| CONVERGE | Сужение до 2-3 финалистов, trade-off анализ | Trade-off matrix |
| DECIDED | Вадим фиксирует решение | Decision record |
| MIGRATED | Решение перенесено в knowledge base | Link to target artifact |

## Структура файлов

```
work/topics/
└── YYYY-MM-DD_slug/
    ├── TOPIC.md              ← topic card + decision record (append-only)
    ├── positions.md          ← лог позиций и аргументов (append-only)
    └── evidence/             ← supporting materials (optional)
        ├── research.md
        └── data.json
```

Формат следует `artifact_centric_interface` Layer 1: файлы на диске, git = версионирование, markdown + структура.

## TOPIC.md — единый документ темы

```markdown
---
id: DLB-NNN
status: OPEN | EXPLORE | CONVERGE | DECIDED | MIGRATED
opened: YYYY-MM-DD
decided: YYYY-MM-DD | null
question: "Одно предложение: что решаем?"
stakes: "Что будет если не решим / решим неправильно"
migrated_to: path/to/artifact.md | null
---

# [Question]

## Context
[Почему вопрос возник. Что уже известно. Ограничения.]

## Alternatives
### A: [Name]
- **Суть:** ...
- **За:** ...
- **Против:** ...
- **Evidence:** ...

### B: [Name]
...

## Trade-offs
| Критерий | Alt A | Alt B | Alt C |
|----------|-------|-------|-------|
| ...      | ...   | ...   | ...   |

## Decision
**Выбрано:** [Alternative X]
**Решил:** Вадим
**Дата:** YYYY-MM-DD
**Обоснование:** [1-3 предложения: почему именно это]
**Отвергнуто:** [Alternatives Y, Z — почему]

## Migration
**Куда:** [path to knowledge artifact]
**Что изменено:** [какой файл создан/обновлён]
**Дата:** YYYY-MM-DD
```

## positions.md — лог обсуждения (append-only)

```markdown
# Positions Log: [Topic]

## Round 1 — YYYY-MM-DD
**Инициатор:** Вадим / Claude
**Позиция:** [суть]
**Аргументы:** [список]
**Evidence:** [ссылки, данные]

## Round 2 — YYYY-MM-DD
**Инициатор:** Claude
**Тип:** COUNTER / REFINEMENT / NEW_ALTERNATIVE
**Позиция:** [суть]
**Аргументы:** [список]
**Реакция на Round 1:** [что учтено]
```

Merge policy: **APPEND_ONLY** (следуя `append_only_audit`). Раунды никогда не удаляются — только добавляются новые.

## Протокол по фазам

### 1. OPEN

Триггер: Вадим формулирует вопрос или кидает тему.

1. Создать `work/topics/YYYY-MM-DD_slug/`
2. Заполнить TOPIC.md: question, context, stakes
3. Предложить 2-5 начальных альтернатив (если Вадим не задал сам)
4. Спросить Вадима: "Что упустил? Какие ещё варианты?"

**Контекст:** используй подход из `doc-coauthoring` Stage 1 (Context Gathering) — уточняющие вопросы, info dump, закрытие пробелов. НЕ дублируй логику — ссылайся на скилл.

### 2. EXPLORE

Для каждой альтернативы:

1. Собрать аргументы ЗА и ПРОТИВ
2. Найти evidence (данные, прецеденты, research)
3. Записать в positions.md как раунды

**Для сложных тем** — применяй `adversarial_reflection`:
- **Analyst pass:** "Какие аргументы ЗА эту альтернативу? Что упущено?"
- **Reviewer pass:** "Какие риски? Что не подтверждено evidence?"
- НЕ дублируй механику — запускай по паттерну из cross_agent

**Для тем, требующих исследования** — делегируй через `deep-research` или `deep-research-gemini` скилл. Результаты → `evidence/`.

### 3. CONVERGE

1. Построить trade-off matrix по ключевым критериям
2. Отсечь явно проигрышные альтернативы (с обоснованием)
3. Для 2-3 финалистов: явная формулировка trade-off
4. Предложить Вадиму рекомендацию с обоснованием
5. **Спросить решение** — решение всегда за Вадимом

### 4. DECIDED

Когда Вадим говорит "делаем X" / "вариант A" / принимает решение:

1. Заполнить секцию Decision в TOPIC.md
2. Зафиксировать: что выбрано, почему, что отвергнуто
3. Поставить `status: DECIDED`

**Формат решения** вдохновлён `human_review_digest` — но проще: вместо 1-5 шкалы используем бинарное ACCEPTED/REJECTED per alternative. НЕ дублируй scoring механику — она здесь не нужна.

### 5. MIGRATED

Решение должно жить в knowledge base, а не в deliberation log:

1. Определить целевой файл:
   - Архитектурное решение → `knowledge/` (entities, policies)
   - Методологическое → `knowledge/playbook/` (patterns, workflows)
   - Инструментальное → `tools/` (scripts, skills)
   - Бизнес-правило → `knowledge/` (business_model, processes)
2. Создать/обновить целевой файл
3. Добавить ссылку на deliberation: `<!-- Decision: DLB-NNN -->`
4. Заполнить Migration секцию в TOPIC.md
5. Поставить `status: MIGRATED`

**Направление миграции:** следует `knowledge_layers` — знание мигрирует ВНИЗ по слоям стабильности (instance → strategy → domain → infrastructure).

## Пересечения с существующими паттернами

### Что ПЕРЕИСПОЛЬЗУЕТСЯ (не дублируется)

| Компонент | Из какого паттерна | Как используется |
|-----------|-------------------|-----------------|
| Context Gathering | `doc-coauthoring` Stage 1 | Фаза OPEN: те же вопросы и info dump |
| Opposing objectives | `adversarial_reflection` | Фаза EXPLORE: Analyst/Reviewer для сложных тем |
| Append-only log | `append_only_audit` | positions.md: раунды никогда не удаляются |
| Files as SoT | `artifact_centric_interface` Layer 1 | Файлы на диске, git = audit trail |
| Knowledge migration | `knowledge_layers` | Фаза MIGRATED: решения текут вниз по слоям |
| Deep research | `deep-research-gemini` skill | Фаза EXPLORE: делегация исследования |

### Что НОВОЕ (не существовало)

| Компонент | Зачем |
|-----------|-------|
| Topic lifecycle (OPEN→MIGRATED) | Ни один паттерн не трекает жизненный цикл вопроса/решения |
| Decision record format | ADR-подобная фиксация: что решили, почему, что отвергли |
| Trade-off matrix | Явное сравнение альтернатив по критериям |
| Decision → knowledge migration | Протокол переноса решений в базу знаний |
| Deliberation index | Каталог всех обсуждений для поиска "почему так решили" |

### Что ПОХОЖЕ, но отличается

| Этот паттерн | Похожий паттерн | Отличие |
|-------------|----------------|---------|
| positions.md (дискуссия) | `analysis_review_merge` (extraction) | ARM извлекает facts из sources; здесь — генерация и сравнение opinions |
| Decision record | `human_review_digest` (feedback) | HRD = scoring готовых items; здесь = выбор между альтернативами |
| Trade-off matrix | `adversarial_reflection` (merge) | AR даёт accept/reject; здесь = multi-criteria comparison |

## Интеграция с другими workflows

### → Backlog

Если решение порождает задачи:
- Стратегическая → `backlog_management` Tier 1 (Strategy)
- Техническая → ops-bot `improvements/backlog.yaml`

### → Meeting insights

Если решение обсуждалось на встрече → ссылка на транскрипт в evidence/.

### → Hive

Если решение затрагивает другие агенты → сообщение через Hive (только после валидации Вадимом).

## Индекс обсуждений

Файл `work/topics/INDEX.md` (DERIVED — регенерируется):

```markdown
# Deliberation Index

| ID | Date | Question | Status | Decision | Migrated to |
|----|------|----------|--------|----------|-------------|
| DLB-001 | 2026-03-07 | Gemini offload architecture | MIGRATED | Flash+Pro hybrid | knowledge/playbook/gemini_offload.md |
| DLB-002 | 2026-03-07 | Discussion pattern | DECIDED | Create pattern | knowledge/playbook/pattern_deliberation.md |
```

## Пример: быстрая дискуссия (1 сессия)

Для простых решений (2-3 альтернативы, можно решить за 5 минут) полный workflow избыточен. Допустимо:

1. Обсудить в чате
2. Зафиксировать решение inline в TOPIC.md (skip positions.md)
3. Мигрировать в knowledge base

Файловая структура всё равно создаётся — для audit trail и поиска "почему так решили".

## Пример: глубокая дискуссия (несколько сессий)

1. Сессия 1: OPEN + начало EXPLORE (alternatives, initial positions)
2. Сессия 2: EXPLORE продолжение (research, adversarial reflection)
3. Сессия 3: CONVERGE → DECIDED → MIGRATED

Между сессиями: TOPIC.md с `status: EXPLORE` — следующая сессия продолжает с того же места (checkpoint через файл, не через контекст).
