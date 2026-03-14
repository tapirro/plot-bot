---
id: georgian-re-reference
type: guide
status: active
owner: vadim
updated: 2026-03-15
domain: real-estate
origin: assistant-awr
confidence: validated
basis: "applied to 2 real deals (Olympus Residence, Kveda Sameba)"
---

# Georgian Real Estate — Reference

> Transferred from assistant AWR. Validated through real deal analysis.

---

## Data Source Hierarchy

| # | Source | What it gives | Access | Status |
|---|--------|--------------|--------|--------|
| 1 | **NAPR API** | Ownership, mortgages, transactions | `napr_lookup.py` — no auth | ✅ Working |
| 2 | **ArcGIS MapServer** | Parcel geometry, area, UNIQ_CODE | `arcgis_spatial.py` — HTTP only | ✅ Working |
| 3 | **GeoServer WFS** | Coordinate → cadastral code | `wfs_cadastre.py` — no auth | 🟡 New (needs layer discovery) |
| 4 | **Place.ge** | Listings: price, area, location | `place_ge_scraper.py` — XHR | ✅ Working |
| 5 | **SS.ge** | Listings (40K DAU, main platform) | Official API exists, needs contact | 🔴 Blocked (awaiting Vadim) |
| 6 | **Myhome.ge** | Listings | Anti-bot 403, needs headless | 🔴 Not started |
| 7 | **GitHub scraper** | Myhome.ge ready-made | `github.com/GiorgiModebadze/Real_estate_market_scrape` | 🟢 Available |
| 8 | **matsne.gov.ge** | Legislation, zoning, building codes | WebFetch works | 🟢 Available |
| 9 | **Copernicus Sentinel-2** | Satellite imagery (10m resolution) | Free, account needed | 🟡 Not integrated |

### WFS — The Missing Bridge

Place.ge gives coordinates. NAPR needs cadastral codes. **WFS bridges this gap:**
```bash
# Point → cadastral code
python3 tools/scripts/wfs_cadastre.py 41.7732 41.7286

# Batch from Place.ge JSON
python3 tools/scripts/wfs_cadastre.py --batch work/data/place_ge_coastal.json

# Discover available layers first
python3 tools/scripts/wfs_cadastre.py --layers
```

**Pipeline:** Place.ge listing → `wfs_cadastre.py` (coords→cadastral) → `napr_lookup.py` (cadastral→ownership) → `land_db.py` (store + score)

---

## Cadastral Code Format

```
ZZ.SS.QQ.PPP.BB.TT.UUU
│  │  │  │   │  │  └── Unit (apartment/room)
│  │  │  │   │  └── Tower/entrance
│  │  │  │   └── Building
│  │  │  └── Parcel (plot)
│  │  └── Quarter
│  └── Sector
└── Zone (e.g. 05 = Adjara)
```

**UNIQ_CODE** = 9 digits without dots: `05.32.12.220` → `053212220`

**API endpoints:**
- NAPR registration: `POST https://naprweb.reestri.gov.ge/api/search` body: `{"cadcode": "XX.XX.XX.XXX.XX.XX.XXX"}`
- ArcGIS parcels: `http://gisappsn.reestri.gov.ge/ArcGIS/rest/services/CadRepGeo/MapServer/14` (Adjara)
- ArcGIS buildings: Layer 12
- **GeoServer WFS:** `http://nv.napr.gov.ge/geoserver/wfs` (coordinate queries)
- **GeoServer WMS:** `http://nv.napr.gov.ge/geoserver/wms` (GetFeatureInfo fallback)

**ВАЖНО:** maps.napr.gov.ge и maps.gov.ge — JavaScript SPA. WebFetch возвращает пустой HTML. Использовать API напрямую.

---

## Confidence Classification (C1–C3)

Apply to every claim in analysis. This is epistemological hygiene.

| Level | Description | Example | Action |
|-------|------------|---------|--------|
| **C3** | Verified | Cadastral extract, closed transaction, state registry | Trust |
| **C2** | Checkable | Seller's spreadsheet with numbers, active listings | Cross-reference |
| **C1** | Marketing | Brochure, verbal claims, "no analogues" | Verify or discount |

**Matrix:**
```
              High value    Medium value    Low value
C3            INVESTIGATE   CONSIDER        PASS
C2            DUE DILIGENCE CONSIDER        PASS
C1            VERIFY FIRST  LOW PRIORITY    PASS
```

When scoring parcels, tag each data point with confidence level. A C1 price is worth less than a C3 transaction price.

---

## Tax Regime (Georgia, updated 2026-03)

| Event | Tax | Notes |
|-------|-----|-------|
| **Purchase** | 0% | No transfer tax in Georgia |
| **Ownership** | ≤1%/year | Of market value |
| **Sale (individual)** | 5% of profit | De facto often not enforced |
| **Sale >6 units/person/year** | 20% | Tax authority classifies as business |
| **Sale (via company)** | 15% corporate | |
| **Rental (non-resident, Virtual Zone)** | 5% | |
| **Rental (standard)** | 20% | |

**Optimization scheme:** Register across 3-4 individuals, ≤6 sales/year each → 5% not 20%.

**Form:** Direct sale via Ministry of Justice (not assignment) — when cadastral codes exist.

> Always verify via matsne.gov.ge — tax rules change.

---

## Foreign Ownership Restrictions

- **Agricultural land:** Forbidden for foreign individuals. OK through Georgian LLC.
- **Non-agricultural:** No restrictions for foreigners.
- **Workaround:** Register a Georgian LLC (ООО), buy land through it. Cost: ~$200-500 registration.
- **Conversion:** Agricultural → non-agricultural possible but bureaucratic (local municipality approval).

---

## Data Quality Rules

1. **Listing prices ≠ transaction prices.** Always note the gap (typically 10-30% in Georgia).
2. **Area in listings vs cadastral area** — always cross-check. Listings often round up.
3. **"Sea view" / "first line"** — verify via satellite/ArcGIS. Marketing ≠ geography.
4. **Year of data matters.** Market can shift 20-30% YoY in Batumi.
5. **Zone names vary** between sources (Place.ge uses informal names, NAPR uses official).
6. **Verify every claim** at the highest available confidence level before acting.
