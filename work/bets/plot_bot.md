---
id: bet-plot-bot
type: bet
title: "Plot Bot — AI Real Estate Developer"
domain: plot-bot
status: proposed
horizon: 2026-09
owner: vadim
created: 2026-03-14
source: voice memo (агент девелопер.mp3)
meanings: [value, elegance, awareness]
---

# Plot Bot — AI Real Estate Developer

## Гипотеза

AI-агент ($200/мес на Claude Code) может автономно построить полный девелоперский бизнес на грузинском побережье: земельный анализ, юридика, финансы, маркетинг, управление инвестпулом. При целевом ROI 10x на землю ($20-30 → $200-350/м²) и кластерном подходе — самоокупаемая машинка.

## Дальний горизонт

Система кластеров элитной недвижимости на побережье Грузии:
- Полная кадастровая база с AI-оценкой каждого участка
- Инвестиционный пул с постоянным притоком инвесторов
- Банковские кредитные линии для ликвидности
- Экосистема сервисов (ресторан, покер-рум, медицина, озеленение)
- Корпоративное ядро Mantissa

## Инварианты

1. В зоне видимости ничего некрасивого
2. Все стройки только через Архнадзор (доверенные архитекторы)
3. Положительный кэш-баланс всегда
4. Покупаем сильно ниже рынка (10x потенциал)

## Ближайшие шаги

- [ ] Сформулировать полный набор инвариантов
- [ ] Просчитать арифметику: кредит → оформление → продажа → закрытие
- [ ] Собрать контакты риэлторов через Сашку и Владимира
- [ ] Поговорить с Максимом (поднять уровень)
- [ ] Прототип: кадастровый анализ одного района (Григолети?)
- [ ] Юридический playground (концепт Игоря) — для соседей/администрации
- [ ] Упаковать оффер для Сашки

## Люди

| Имя | Роль | Статус |
|-----|------|--------|
| Вадим | Стратегия, инициатор | Active |
| Игорь Головченко | Инвестор (C-level) | Potential |
| Игорь Тен | Инвестор | Potential |
| Владимир | Инвестор, знает риэлторов | Potential |
| Ника | Озеленение/ландшафт | Potential |
| Максим | Needs leveling up | Potential |
| Сашка | Риэлторы, контакты, шоу | Potential |
| Олег | Мебель, шоу-рум | Potential |
| Доктор Данила | Медицина ($250/приём) | Potential |

## Metrics

| Metric | Current | Target | Method |
|--------|---------|--------|--------|
| Cadastral coverage (target zones) | ~5% | 80% | ArcGIS + NAPR scrapers |
| Data sources integrated | 2 | 6 | Scraper pipeline |
| Unit economics validated | 1 model | 3 scenarios | Financial modeling |
| Investor-ready materials | 0 | 3 docs | Analysis + design |
| Avg cycle impact | 4.0 | ≥3.5 | VERA self-scoring |

## Next Experiment

**Hypothesis:** Automated land scoring model (cadastral + pricing + location factors) can identify undervalued plots with 5x+ potential in Grigoleti zone.

**Method:** Build SQLite DB with ArcGIS parcels + Place.ge listings + NAPR transactions. Score each parcel on 8 factors. Validate top-10 against manual assessment.

**Success criteria:** ≥3 parcels in top-10 that Vadim confirms as worth investigating.

**Timeline:** Mega-cycle 2 (cycles 6-9).

## Суб-домены

- `plot-bot/land` — кадастровый анализ, база земли, факторы ценообразования
- `plot-bot/legal` — контракты, Архнадзор, юридическое обоснование
- `plot-bot/finance` — арифметика, кредитные линии, P&L, инвестпул
- `plot-bot/marketing` — сайты, лендинги, партнёрство с банками (TBC Concept)
- `plot-bot/services` — ресторан, медицина, озеленение, спорт, агрорынок
