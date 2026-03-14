---
id: analysis-coastal-data-001
type: analysis
title: "Coastal Land Data Collection — First Harvest"
domain: plot-bot
status: active
created: 2026-03-14
sources: [place.ge, arcgis-napr]
---

# Coastal Land Data — First Harvest (2026-03-14)

## Summary

First systematic data collection for Adjara/Guria coastal zone. Two sources: Place.ge listings (market asking prices) and NAPR ArcGIS (cadastral parcels with area/purpose).

## Place.ge Listings

**Source:** `place_ge_coastal.json` | 30 coastal land listings (for sale)

### Location Breakdown
| City | Count |
|------|-------|
| Batumi | 10 |
| Ozurgeti | 10 |
| Ureki | 6 |
| Kobuleti | 3 |
| Sarpi | 1 |

### Price Distribution (listings with $/m2 data: 28)
- Average: $487/m2 (skewed by luxury Batumi plots)
- **Under $30/m2 (target range): 5 listings** — all Ozurgeti municipality
- Under $100/m2: 10 listings — Ozurgeti + Ureki
- Under $200/m2: 19 listings

### Target-Range Listings (<$100/m2)
| $/m2 | Area | Location | Notes |
|------|------|----------|-------|
| $1 | 5,000 m2 | Ozurgeti, ბახვი | [unverified: suspiciously low] |
| $3 | 130,000 m2 | Ozurgeti, ლაითური | [unverified: may be agricultural] |
| $3 | 130,369 m2 | Ozurgeti | [unverified: may be agricultural] |
| $7 | 3,000 m2 | Ozurgeti, ნატანები | ⚠️ Natanebi — near Ureki corridor |
| $8 | 186,525 m2 | Ozurgeti, გურია | [unverified: 18.6 ha, likely agricultural] |
| $9 | 1,000 m2 | Ureki | Small but in target area |
| $14 | 3,000 m2 | Ozurgeti, ნატანები | Natanebi — near Ureki corridor |
| $40 | 600 m2 | Ozurgeti, ნატანები | Small plot |
| $49 | 7,190 m2 | Ozurgeti | Good size, needs location check |
| $55 | 5,172 m2 | Ureki | **Interesting: right area, right size** |

**Key insight:** Ozurgeti municipality (which includes Ureki, Grigoleti, Natanebi) has the cheapest coastal land. The $7-55/m2 range in Natanebi-Ureki corridor aligns with our $20-30/m2 target.

### Data Quality
- Price data: 28/30 listings have $/m2 [verified: 2 sources within listing]
- Location data: all 30 have location [verified: Place.ge provides]
- Area data: all 30 have area [verified: Place.ge provides]
- ⚠️ No cadastral IDs — cannot cross-reference with NAPR
- ⚠️ Distance to sea unknown — needs geocoding
- ⚠️ $1-$8/m2 listings likely agricultural land or errors — needs verification

## ArcGIS Cadastral Data

### Ureki Zone
**Source:** `arcgis_ureki.json` | 1,816 parcels | bbox: (41.72, 41.75, 41.77, 41.78)

| Metric | Value |
|--------|-------|
| Total parcels | 1,816 |
| Area range | 6 — 336,148 m2 |
| Median area | 1,850 m2 |
| Target size (1K-50K m2) | 1,230 (68%) |
| Large (>5K m2) | 219 (12%) |

Purpose codes: 1 (466), 2 (160), Unknown (1,190)

### Grigoleti-Ureki Wide Zone
**Source:** `arcgis_grigoleti_wide.json` | 2,724 parcels | bbox: (41.65, 41.72, 41.75, 41.78)

| Metric | Value |
|--------|-------|
| Total parcels | 2,724 |
| Area range | 0 — 336,148 m2 |
| Median area | 1,095 m2 |
| Target size (1K-50K m2) | 1,453 (53%) |
| Large (>5K m2) | 209 (8%) |
| Very large (>10K m2) | 87 |

**Cluster-candidate parcels (>20,000 m2):**
| Cadastral Code | Area |
|----------------|------|
| 204606236 | 336,148 m2 (33.6 ha) |
| 204801304 | 136,705 m2 (13.7 ha) |
| 204606016 | 99,929 m2 (10.0 ha) |
| 204804215 | 90,599 m2 (9.1 ha) |
| 204606344 | 89,757 m2 (9.0 ha) |
| 204606454 | 81,936 m2 (8.2 ha) |
| 204803630 | 55,335 m2 (5.5 ha) |
| 204606024 | 47,274 m2 (4.7 ha) |
| 204805418 | 42,926 m2 (4.3 ha) |
| 204606301 | 38,360 m2 (3.8 ha) |

**Key insight:** 87 parcels over 1 hectare in the Grigoleti-Ureki zone. The 20K-50K m2 range is ideal for our cluster model (unit economics modeled on 20,000 m2).

### Data Quality
- Area: all parcels have area [verified: NAPR official]
- Cadastral codes: all have UNIQ_CODE [verified: NAPR]
- Purpose codes: 60% unknown — need NAPR lookup to decode
- ⚠️ No price data — ArcGIS has no pricing
- ⚠️ No ownership data — needs napr_lookup.py per-parcel
- ⚠️ Geometry excluded (--no-geometry) to save space — re-query with geometry for mapping

## Cross-Reference Gap

**Critical gap:** No way to link Place.ge listings to ArcGIS parcels.
- Place.ge: has prices, no cadastral IDs
- ArcGIS: has cadastral IDs, no prices
- Bridge: need geocoding (lat/lon from Place.ge listing pages) or manual matching

## Next Steps (for scoring model + future cycles)

1. Run `napr_lookup.py` on top 20 large parcels — check ownership, mortgages, purpose
2. Decode purpose codes (1, 2, UNKNOWN) via NAPR documentation
3. Geocode Place.ge listings to get coordinates → spatial join with ArcGIS
4. Build scoring model using: area, distance to sea, purpose, price/m2, access road
5. Expand scraping: run full Place.ge (all pages, not just 10) + Myhome.ge when possible
