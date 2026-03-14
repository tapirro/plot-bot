---
name: analyze-deal
description: Analyze a real estate deal proposal. Creates structured case folder, runs market research, produces VERDICT and QUESTIONS deliverables.
  TRIGGER when: user shares a real estate offer, property listing, investment proposal, or land/apartment deal — via link, file, chat export, PDF, or text description. Look for signals: price per m2, property location, developer name, "предложение", "участок", "квартира", "объект", "инвестиция в недвижимость", "сделка", square meters, ROI projections.
  DO NOT TRIGGER when: user asks general questions about real estate market without a specific deal, discusses existing analyzed deals, or references deals only in passing.
argument-hint: "[path to materials, link, or description]"
allowed-tools: Agent, Bash, Glob, Grep, Read, Write, Edit, WebSearch, WebFetch
context: fork
agent: general-purpose
---

Analyze real estate deal from: $ARGUMENTS

## Mission

Получено предложение по недвижимости. Создать структурированный кейс, провести исследование, выдать VERDICT + QUESTIONS по паттерну `knowledge/playbook/pattern_real_estate_analysis.md`.

## Protocol

### Phase 1: INTAKE (1-2 мин)

1. **Прочитать паттерн:**
   ```
   Read knowledge/playbook/pattern_real_estate_analysis.md
   ```

2. **Определить тип сделки** из входных данных: LAND | APARTMENT | HOUSE | COMMERCIAL | DEVELOPMENT | FUND

3. **Создать папку кейса:**
   ```bash
   DATE=$(date +%Y-%m-%d)
   SLUG="<slug-from-name>"
   mkdir -p work/intake/deals/${DATE}_${SLUG}/{source/docs,source/photos,research,analysis,questions}
   ```

4. **Собрать материалы в `source/`:**
   - Если Telegram-экспорт → `python3 tools/scripts/telegram_export.py /path/ --summary` для обзора, `--copy-to source/photos/` для фото
   - Если чат → нормализовать в `source/chat.md`
   - Если PDF/файлы → скопировать в `source/docs/`
   - Если фото → скопировать в `source/photos/`
   - Если ссылки → создать `source/links.md`

5. **Прочитать все материалы** (PDF через Read, фото мультимодально) → первичное понимание

6. **Заполнить README.md** по шаблону `tools/templates/deal_readme.md`

### Phase 2: RESEARCH (3-5 мин, параллельно)

Запустить 3 субагента параллельно:

**Субагент 1: Market Research** (general-purpose)
```
Prompt: Исследовать рыночные цены недвижимости в {локация} для типа {deal_type}.
Искать:
- "{город} недвижимость цены {тип} {год}"
- "{city} real estate price per sqm {type} {year}"
- "{название проекта} отзывы"
Результат: сохранить в research/market_data.md
```

**Субагент 2: Legal Research + Кадастр** (general-purpose)
```
Prompt: Исследовать юридические аспекты покупки {deal_type} в {страна/город}.
Искать:
- Налоговый режим для иностранцев
- Процедура оформления
- Ограничения для нерезидентов

Если Грузия и есть кадастровые коды:
  python3 tools/scripts/napr_lookup.py <коды> > research/cadastral.json
  python3 tools/scripts/napr_lookup.py --parcel <UNIQ_CODE> >> research/cadastral.json
НЕ использовать WebFetch на maps.napr.gov.ge (JavaScript SPA, не работает).

Результат: сохранить в research/legal_notes.md + research/cadastral.json
```

**Субагент 3: Document Analysis** (general-purpose)
```
Prompt: Проанализировать документы из source/docs/ и source/photos/.
Извлечь: ключевые цифры, красные флаги, несоответствия.
Результат: краткое саммари для анализа.
```

### Phase 3: ANALYSIS (3-5 мин)

1. **Прочитать все research/ файлы**
2. **Заполнить 6 осей** (VALUE, LOCATION, RISK, CREDIBILITY, LIQUIDITY, URGENCY) с обоснованиями и confidence-маркерами
3. **Сценарный анализ** (если инвестиция): оптимистичный / базовый / пессимистичный
4. **Красные флаги** — пронумерованный список с [C1]/[C2]/[C3]
5. **Вопросы к продавцу** — ТОП-10 с приоритетами [CRITICAL]/[IMPORTANT]/[NICE-TO-HAVE]

### Phase 4: DELIVERABLES

1. **Создать `analysis/VERDICT.md`** по шаблону `tools/templates/deal_verdict.md`
2. **Создать `questions/QUESTIONS.md`** по шаблону `tools/templates/deal_questions.md`
3. **Обновить `README.md`** — заполнить axes, confidence, обновить status → `analysis`

### Phase 5: PRESENT

Показать Вадиму компактно:

```
## {Название} — Radar

| Ось | Балл |
|-----|------|
| ... | .../5 |

**Средний:** X.X/5 | **Confidence:** C{N} | **Срочность:** E{N}

## Вердикт
{1-3 предложения}

## ТОП-3 вопроса
1. ...

Полный анализ: `work/intake/deals/{slug}/analysis/VERDICT.md`
Вопросы продавцу: `work/intake/deals/{slug}/questions/QUESTIONS.md`
```

## Key Rules

- **Каждый claim** в VERDICT должен иметь маркер confidence [C1]/[C2]/[C3]
- **Листинговые цены ≠ цены сделок** — всегда отмечать разницу
- **Не додумывать** — если данных нет, писать "данные отсутствуют" и добавлять в QUESTIONS
- **Грузинская специфика** — проверять по reference-секции в паттерне
- **Комиссии** — всегда считать долю от валовой прибыли, не только % от оборота
