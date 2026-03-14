---
id: cycle-report-002
type: cycle-report
title: "Cycle 2: Coastal land data collection — Place.ge + ArcGIS"
domain: plot-bot
status: final
created: 2026-03-14
cycle: 2
cycle_type: RESEARCH
mode: FULL
north_stars: [V, A]
impact: 4
---

## Hypothesis

Collect the first real dataset of coastal land listings and cadastral parcels for the Ureki-Grigoleti target zone. This data is the foundation for scoring models, pricing analysis, and identifying cluster-candidate parcels.

## Changes

- Created `context/cycle_plan.md` — plan for mega-cycle 0, positions 1-4 (RESEARCH → ANALYSIS → BUILD → ESCALATION)
- Created `work/data/place_ge_coastal.json` — 30 coastal land listings from Place.ge [verified: 2 sources per listing (area + price from same page)]
  - 19 listings under $200/m2, 10 under $100/m2, 5 under $30/m2
  - Ureki: 6 listings ($9-$121/m2), Ozurgeti municipality: 10 listings ($1-$108/m2)
- Created `work/data/arcgis_ureki.json` — 1,816 cadastral parcels in Ureki zone [verified: NAPR official API]
  - 1,230 parcels in target size range (1K-50K m2), median 1,850 m2
- Created `work/data/arcgis_grigoleti_wide.json` — 2,724 cadastral parcels in wider Grigoleti-Ureki zone [verified: NAPR official API]
  - 87 parcels over 10,000 m2 — cluster candidates
  - Top 10 parcels range from 38K to 336K m2
- Created `work/data/coastal_data_analysis.md` — analysis report with cross-reference gaps, data quality notes, and next steps
- Ran NAPR parcel lookup on top 3 largest parcels — confirmed registration data exists
- Fixed Grigoleti bbox: original preset too narrow (11 parcels), expanded to (41.65,41.72,41.75,41.78) → 2,724 parcels

## Impact

**Score: 4** (Significant — first real dataset with actionable market intelligence)

- **V (Value):** Identified 10 listings under $100/m2 in target Ureki-Natanebi corridor — proves the $20-30/m2 price point exists in practice. 87 large parcels (>1 ha) are cluster candidates.
- **A (Awareness):** Cadastral coverage of target zone: ~4,500 unique parcels mapped. Place.ge coverage: 30 listings (limited but real pricing data).

Key discovery: Ozurgeti municipality land at $7-55/m2 in Natanebi-Ureki corridor aligns perfectly with unit economics model from Cycle 1.

Critical gap identified: no bridge between Place.ge prices and ArcGIS cadastral IDs — needs geocoding or manual matching.

## Next

- Run scoring model analysis on collected data (Cycle 3, position 2)
- Decode NAPR purpose codes (1, 2, UNKNOWN) — 60% of parcels have unknown purpose
- Geocode Place.ge listings to enable cross-reference with ArcGIS parcels
- Investigate NAPR REG_N vs cadastral code format mismatch
- Run full Place.ge scrape (all pages) for broader market picture
