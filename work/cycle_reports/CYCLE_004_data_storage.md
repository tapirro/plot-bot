---
id: cycle-report-004
type: cycle-report
title: "Cycle 4: Data storage — SQLite schema + ETL"
domain: plot-bot
status: final
created: 2026-03-14
cycle: 4
cycle_type: BUILD
mode: FULL
north_stars: [E, R]
impact: 4
---

## Hypothesis

A unified SQLite database for listings (Place.ge) and parcels (ArcGIS) will enable structured querying, scoring pipeline integration, and repeatable ETL — replacing ad-hoc JSON file reads with indexed, queryable storage.

## Changes

- Created `tools/scripts/land_db.py` — SQLite schema + ETL + query CLI
  - **Schema:** 4 tables — `listings`, `parcels`, `scores`, `listing_parcel` (many-to-many link)
  - **listings:** composite PK (source, id), normalized zone/city fields, UPSERT on reload
  - **parcels:** PK uniq_code (9-digit cadastral), precomputed compactness metric (4*pi*area/perimeter^2)
  - **scores:** full scoring model schema (7 filters + 5 category scores + total + class), supports remote/enriched/verified modes
  - **listing_parcel:** match_method + confidence for linking listings to cadastral parcels
  - **Indexes:** zone, price, area, score_class, total_score for fast filtering
- ETL loaded existing data: **30 listings + 3,939 parcels** (440 deduped across overlapping ArcGIS exports)
- CLI commands: `init`, `load-listings`, `load-parcels`, `load-all`, `stats`, `query`, `export`
- Zone normalization handles Georgian + English location names
- Query filter validated: 6 listings pass F1 (≤$40/m²) + F2 (≥3,000m²) screening

## Impact

**Score: 4 — Significant.** New reusable infrastructure (E) that enables the scoring pipeline. Schema directly maps to scoring_model.md categories. Compactness precomputation (R) means S3 shape factor is query-ready for all 3,939 parcels. The `query` command already surfaces deal candidates matching scoring filters.

## Next

- Build `land_scorer.py` — implement scoring formulas from scoring_model.md, write scores to DB
- Batch score all 30 listings in remote mode
- Add coastline distance calculation (haversine from parcel centroid to nearest coast point)
- SS.ge API access escalation (Cycle 4 task)
