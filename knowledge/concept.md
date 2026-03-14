---
id: overview-concept
type: charter
status: active
owner: vadim
updated: 2026-03-13
domain: assistant/awr
---

# Концепт

AWR (Agent Workbench Repository) — операционная среда для Claude Code агентов. Не хранилище файлов, а **рабочая мастерская** с формализованными артефактами, инструментами обнаружения и визуализацией.

## Философия

Агент — не чат-бот. Это **операционная среда**, которая:

- Имеет доступ к рабочим системам (Fireflies, Hive, Jira, BigQuery)
- Знает контекст: компании, людей, проекты, приоритеты
- Действует по правилам, но умеет импровизировать
- Накапливает знания итеративно — от сырого к структурированному
- **Визуализирует** состояние мастерской через дашборды в реальном времени

## Принципы

1. **Iterate fast** — не формализуем раньше, чем нужно. Сначала работает, потом красиво.
2. **Single source of truth** — каждый факт живёт в одном месте.
3. **Tools over docs** — лучше рабочий скрипт, чем описание процесса.
4. **Reusable mechanics** — удачные решения ретранслируем в другие AWR.
5. **Formalized artifacts** — каждый документ имеет тип, схему и контракт блоков.
6. **Queryable knowledge** — знания доступны через `./ask`, не через grep.

## Архитектура

```
          ┌─────── UI (Dashboards) ───────┐
          │  navigator · strategy · xray  │
          └──────────────┬────────────────┘
                         │ /api/...
               ┌─────────┴─────────┐
               │    serve.py (API)  │
               └─────────┬─────────┘
                    │    │    │
              ┌─────┘    │    └──────┐
              ▼          ▼           ▼
         ┌────────┐ ┌────────┐ ┌─────────┐
         │ ./ask  │ │ SQLite │ │ OAuth/  │
         │(Go bin)│ │  (xray)│ │ Ext API │
         └───┬────┘ └────────┘ └─────────┘
             ▼
     ┌───────────────────────┐
     │  Формализованные      │
     │  артефакты (YAML+MD)  │
     │  knowledge/ · work/   │
     └───────────────────────┘
```

### Слои

| Слой | Что | Компоненты |
|------|-----|------------|
| **Артефакты** | Формализованные знания | YAML-шапка + markdown блоки с ролями |
| **Навигатор** | Обнаружение и валидация | `./ask` — 13 команд, <10мс, JSON/wide/quiet |
| **API** | Мост между ask и UI | `serve.py` — проксирует ask + внешние API |
| **UI** | Визуализация | 10+ дашбордов, Mantissa Design System |
| **Скиллы** | Методологии | `.claude/skills/` — формализованные процессы |

### Артефакты

Каждый артефакт = YAML-шапка (id, type, status, owner, domain, updated) + markdown с именованными блоками (problem, solution, decisions, actions, etc.). Типы: `knowledge/playbook/artifact_types.md`.

### Зоны мастерской

| Зона | Папка | Аналогия |
|------|-------|----------|
| Вход | `in/` | Входящий лоток |
| Знания | `knowledge/`, `knowledge/playbook/` | Полка с книгами |
| Работа | `work/bets/`, `work/topics/`, `work/insights/` | Верстак |
| Инструменты | `tools/scripts/`, `tools/ui/` | Стеллаж с инструментами |
| Выход | `out/deliverables/` | Витрина |
| Система | `.claude/skills/`, `.claude/hooks/` | Пульт управления |

## Кристаллизация домена

AWR — движок кристаллизации домена. Агент, работая в домене (hilart/resale, voic/research), не "документирует" домен отдельным шагом. Его рабочие продукты — insights, patterns, methodologies — и есть формализация домена.

**Кристаллизация:** жидкое знание (разговоры, эксперименты) → insight (draft) → pattern (active) → methodology (mature). Каждая стадия — и рабочий продукт, и единица домена.

### Эмерджентные свойства

1. **Нулевая стоимость формализации.** Формат артефакта = формат мышления агента. Frontmatter = метаданные решения. Blocks = структура рассуждения.
2. **Самоописывающаяся система.** `./ask h` → compliance, gaps, stale, provenance. Система знает свою полноту, пробелы, зрелость.
3. **Три проекции одного источника.** Артефакт: агенту → `./ask f`, человеку → dashboard, сети → Hive API (`./ask @contract -j`).
4. **Прогрессивная кристаллизация.** draft→active→mature для паттернов, proposed→experiment→achieved для ставок.
5. **Покрытие домена измеримо.** `./ask audit` — качество артефактов. `vchain.py coverage` — покрытие цепочек ценности.

### Epistemic Provenance

Каждый артефакт несёт эпистемологию: `confidence` (anecdotal→observed→validated→proven), `origin` (откуда знание), `basis` (на чём основано). Status = lifecycle формата, confidence = достоверность содержания. Спецификация: `knowledge/playbook/style_guide.md`.

## Value Chains и Delivery

### Слоты

AWR определяет контракт агента. Типы артефактов с валидацией = классы слотов. `slot = artifact_type @ target_domain` (пример: `pattern @ voic/research`). Дополнительно — tool slots: skill, script, dashboard, plugin.

### Delivery lifecycle

```
package (out/deliverables/) → deliver (vchain.py deliver) → ack (vchain.py ack) → promote
```

Append-only журнал: `context/delivery_log.jsonl`. Receiver types: `agent:<name>`, `human:<name>`, `system:<name>`. Readiness levels: `draft → tested → shipped`. Promote gate: `vchain.py promote --check` (exit code 0/1).

### Agent Contract

Контракт не пишется — вычисляется из состояния репозитория:

```bash
./ask @contract       # текстовый контракт
./ask @contract -j    # JSON для Hive API
```

Источники: `.claude/agent.json` (identity), индекс артефактов (types, domains), `out/deliverables/` (delivery), value chain graph.

## Дистрибуция

Toolkit: `out/deliverables/awr-toolkit/` — полный пакет для развёртывания AWR в новом репозитории (ask + UI + schema + scaffold + install).
