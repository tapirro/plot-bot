---
id: geographic-focus-kobuleti-poti
type: directive
status: pending
priority: P0
category: strategy
created: 2026-03-15
title: "Geographic Focus: Kobuleti–Poti Corridor, First & Second Line Only"
---

# Geographic Focus: Kobuleti–Poti Corridor, First & Second Line Only

## Directive from Vadim (2026-03-15)

> Сконцентрируем плотбота на сборе лотов строго в районе от Кобулети до Поти (не южнее!). Рассматриваем участки только на первой и второй линии — не далее 200 метров от береговой линии.

## Constraint Definition

### Latitude Bounds (HARD)

| Boundary | Value | Location |
|----------|-------|----------|
| **South limit** | lat ≥ 41.78 | Kobuleti south edge (Kobuleti-S zone bbox_lat_min) |
| **North limit** | lat ≤ 42.20 | Poti north edge (Poti-city zone bbox_lat_max) |

**EXCLUDED** (south of Kobuleti): Tsikhisdziri, Chakvi, Batumi, Gonio, Kvariati, Sarpi, Makhinjauri, Khelvachauri — anything with lat < 41.78.

### Sea Distance (HARD)

| Parameter | Value | Meaning |
|-----------|-------|---------|
| `sea_distance_m` | ≤ 200 | First + second line from coastline |

Use `sea_distance_m` (haversine from nearest coastline point). If `sea_distance_osm_m` is available and differs significantly, use the smaller of the two (conservative — include rather than exclude).

### Corridor Map

```
North  ▲  Poti          (lat ~42.15)  ← INCLUDED
       │  Maltakva      (lat ~42.03)  ← INCLUDED
       │  Grigoleti-N   (lat ~41.91)  ← INCLUDED
       │  Grigoleti     (lat ~41.83)  ← INCLUDED
       │  Ureki         (lat ~41.77)  ← INCLUDED
       │  Shekvetili    (lat ~41.80)  ← INCLUDED
       │  Kobuleti-S    (lat ~41.84)  ← INCLUDED
       │  Kobuleti-N    (lat ~41.79)  ← INCLUDED
       ├──────────────────────────────── lat = 41.78 CUTOFF
       │  Tsikhisdziri  (lat ~41.75)  ✗ EXCLUDED
       │  Chakvi        (lat ~41.73)  ✗ EXCLUDED
       │  Batumi        (lat ~41.64)  ✗ EXCLUDED
South  ▼  Sarpi         (lat ~41.53)  ✗ EXCLUDED
```

## Implementation: What to Change

### 1. `place_ge_scraper.py` — Update COASTAL_CITIES

**Current:**
```python
COASTAL_CITIES = [
    "batumi", "kobuleti", "gonio", "ureki", "chakvi",
    "kvariati", "sarpi", "makhinjauri", "ozurgeti",
]
```

**New:**
```python
COASTAL_CITIES = [
    "kobuleti", "ureki", "ozurgeti", "poti",
]
```

Removed: batumi, gonio, chakvi, kvariati, sarpi, makhinjauri (all south of Kobuleti).
Added: poti (was missing — it's our northernmost target).

### 2. `universe_census.py` — Remove Tsikhisdziri Zone

**Remove this line:**
```python
("Tsikhisdziri",  41.72, 41.72, 41.78, 41.78),   # verified: 7206
```

All other zones (Poti-city through Kobuleti-N) remain — they're within the corridor.

### 3. `scoring_model.md` — Tighten F3 Hard Filter

**Current F3:** `≤3 km from sea`
**New F3:** `≤200 m from sea` (first + second line only)

**Current S2 distance scoring (50% weight):**
```
<500m → 1.0, 500m–1km → 0.8, 1–2km → 0.5, 2–3km → 0.3
```

**New S2 distance scoring (simplified — all parcels are <200m):**
```
<50m → 1.0 (absolute first line)
50–100m → 0.9
100–150m → 0.7
150–200m → 0.5
```

### 4. `land_db.py` — Add Latitude Hard Filter

In `normalize_zone()` and anywhere zone is assigned, add a guard:

```python
if lat is not None and lat < 41.78:
    return None  # South of corridor — exclude
```

### 5. `arcgis_spatial.py` — Add Coastal-Strip Presets

The existing presets are wide boxes. Add narrow coastal-strip presets optimized for the 200m target:

```python
# Narrow coastal strips (~500m wide for safety margin)
"kobuleti-coast": (41.78, 41.82, 41.80, 41.88),
"shekvetili-coast": (41.75, 41.78, 41.78, 41.82),
"ureki-coast": (41.73, 41.75, 41.76, 41.78),
"grigoleti-coast": (41.70, 41.78, 41.73, 41.95),
"poti-coast": (41.65, 42.05, 41.70, 42.18),
```

### 6. CLAUDE.md — Add Geographic Focus Section

Add after existing geographic sections:

```markdown
### Geographic Focus (Iteration 2026-03)

**Corridor:** Kobuleti → Poti (lat 41.78 – 42.20)
**Depth:** First + second line only (≤200m from coastline)
**Hard filter:** `sea_distance_m ≤ 200 AND bbox_lat_min ≥ 41.78`

EXCLUDED zones: Tsikhisdziri, Chakvi, Batumi, everything south of lat 41.78.
Do NOT scrape, score, or analyze parcels outside this corridor.
```

## Data Reality Check

Current DB (2026-03-15):
- 37,914 total parcels
- **46 parcels within 200m** of coast (all at lat ~41.75, Ureki/Shekvetili)
- **0 parcels within 200m north of lat 41.78** (Kobuleti proper and north)
- 22,158 parcels within 3km

**This means:** The bot needs a **focused coastal-strip data collection campaign** before meaningful scoring within the 200m constraint is possible. Priority actions:

1. Run ArcGIS census with narrow coastal-strip bounding boxes (500m wide)
2. Enrich all coastal-strip parcels with sea_distance_m
3. Scrape Place.ge specifically for Kobuleti, Ureki, Poti listings
4. Score only parcels passing both lat ≥ 41.78 AND sea_distance_m ≤ 200

## Execution Sequence

1. **TOOL cycle:** Apply code changes (scraper filter, zone removal, scoring model)
2. **COLLECT cycle (Sonnet):** Run ArcGIS coastal-strip census for Kobuleti–Poti
3. **COLLECT cycle (Sonnet):** Run Place.ge scraper with new COASTAL_CITIES
4. **TOOL cycle:** Bulk sea_distance_m enrichment for new coastal parcels
5. **ANALYZE cycle (Opus):** Score corridor parcels, report coverage gaps

## Success Criteria

| Metric | Current | Target |
|--------|---------|--------|
| Parcels within 200m in corridor | 0 | ≥200 |
| Listings in corridor | ~10 | ≥30 |
| Zone coverage (Kobuleti–Poti) | partial | 100% coastal strip |
| Parcels south of lat 41.78 scored | ~1,600 (batumi) | 0 |
