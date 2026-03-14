---
id: cycle-report-003
type: cycle-report
title: "Cycle 3: Land scoring model"
domain: plot-bot
status: final
created: 2026-03-14
cycle: 3
cycle_type: ANALYSIS
mode: FULL
north_stars: [V, R]
impact: 4
---

## Hypothesis

A weighted scoring model with hard filters can systematically identify undervalued coastal land parcels. By defining 14 pricing factors across 5 categories (value potential, location, parcel, infrastructure, cluster potential) with explicit weights, we can automate screening of 1,800+ parcels and surface the best candidates for cluster development.

## Changes

- Created `work/scoring_model.md` — complete land scoring methodology [verified: grounded in Place.ge + ArcGIS data from Cycle 2]
  - Level 1: 7 hard filters (price, area, sea distance, legal, zoning, aesthetics, access)
  - Level 2: 5-category weighted scoring (S1:30% value, S2:25% location, S3:20% parcel, S4:15% infra, S5:10% cluster)
  - 14 individual factors with formulas, scales, and data source mapping
  - 3 scoring modes: Remote (43% data coverage), Enriched (+satellite), Verified (+site visit)
  - Applied model to 3 sample listings from Place.ge data — validated scoring logic
  - Gap analysis: 6/14 factors available now, 3 partial, 5 require site visit
- Updated `work/bets/plot_bot_roadmap.md` — marked Phase 1.4 scoring tasks and Phase 0.2 criteria task as complete

## Impact

**Score: 4 (Significant)** — New actionable methodology that directly enables deal screening. The model is grounded in real price data (28 listings across 5 price zones) and produces a clear A-F classification. Key insight from analysis: Ureki parcels show 20x price variance ($9-$226/m²) driven primarily by sea distance — confirming location as the #1 factor and justifying the 25% weight. Not a 5 because the model can't yet be run automatically (needs land_scorer.py implementation in a BUILD cycle).

## Next

- BUILD cycle 4: implement `land_scorer.py` with remote mode + batch scoring
- Calculate sea distance for all ArcGIS parcels (coastline coordinates + haversine)
- Run batch scoring on Ureki 1,816 parcels to identify top candidates
- Copernicus DEM integration for slope/terrain data (P2)
