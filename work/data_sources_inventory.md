---
id: task-1.1-data-sources
type: research
title: "Georgian Real Estate Data Sources — Inventory"
domain: plot-bot/land
status: complete
created: 2026-03-14
parent_bet: bet-plot-bot-roadmap
phase: "1.1"
---

# Georgian Real Estate Data Sources — Inventory

## Executive Summary

6 usable data sources identified. 2 already integrated (NAPR, ArcGIS). SS.ge has an official API (contact required). Myhome.ge replaces mymarket.ge for real estate (403 on scraping — anti-bot). Place.ge has 2,132 listings. Satellite imagery free via Copernicus. No public transaction price registry found.

**CORRECTION:** CLAUDE.md lists mymarket.ge as a data source — **mymarket.ge is NOT a real estate platform** (it's general e-commerce like OLX). The real estate arm of the MY.GE group is **myhome.ge**.

---

## Source Matrix

| # | Source | Type | Status | Auth | Land Data | Prices | Coverage | Priority |
|---|--------|------|--------|------|-----------|--------|----------|----------|
| 1 | NAPR API | Registry | ✅ Integrated | None | Ownership, mortgages, transactions | No | All Georgia | — |
| 2 | ArcGIS (NAPR) | GIS | ✅ Integrated | None | Area, floors, geometry | No | All Georgia | — |
| 3 | SS.ge | Marketplace | 🟡 API exists | Contact SS.ge | Listings, area, location | ✅ Ask prices | ~40K daily users | **P0** |
| 4 | Myhome.ge | Marketplace | 🔴 403 (anti-bot) | N/A | Listings, area, location | ✅ Ask prices | Major platform | P1 |
| 5 | Place.ge | Marketplace | 🟡 SPA, scrapable | None | ~2,132 listings | ✅ Ask prices | Medium | P1 |
| 6 | Copernicus Sentinel-2 | Satellite | 🟢 Free API | Free account | 10m resolution imagery | No | Global | P2 |
| 7 | Public Deed Registry | Gov | 🔴 Not found | ? | Transaction history | ❓ Unknown | ? | Research |
| 8 | Zoning/Land Use | Gov | 🔴 Not found | ? | Permitted use, coefficients | No | ? | Research |

---

## Detailed Assessment

### 1. NAPR API ✅ INTEGRATED

**Endpoint:** `POST https://naprweb.reestri.gov.ge/api/search`
**Tool:** `python3 tools/scripts/napr_lookup.py <cadastral_code>`
**Auth:** None

**Data available:**
- Current owner (applicants list)
- Full transaction history (registrations, transfers)
- Mortgage status (active/terminated)
- Registration dates
- Address

**Data NOT available:**
- Transaction prices (not in API response)
- Land area (use ArcGIS for this)
- Zoning / permitted use

**Limitations:**
- Requires exact cadastral code (no search by area/region)
- Rate limiting unknown (0.5s delay built in)
- No batch endpoint — sequential requests only

**Expansion needed:**
- [ ] Batch query by region (enumerate cadastral codes from ArcGIS, then cross-reference)
- [ ] Monitor for ownership changes (delta tracking)

---

### 2. ArcGIS REST API ✅ INTEGRATED

**Base URL:** `http://gisappsn.reestri.gov.ge/ArcGIS/rest/services/CadRepGeo/MapServer/`
**Tool:** `python3 tools/scripts/napr_lookup.py --parcel <9_digit_code>`
**Auth:** None

⚠️ **HTTP only** (not HTTPS)

**Layers:**
- Layer 14: Parcels (area, UNIQ_CODE, geometry)
- Layer 12: Buildings (floors, type)

**Data available:**
- Parcel area (m²)
- Parcel geometry (boundaries)
- Building footprints and floors
- UNIQ_CODE linkage to NAPR

**Expansion needed:**
- [ ] Spatial query: get all parcels within bounding box (for cluster analysis)
- [ ] Export parcel geometries for map visualization
- [ ] Cross-reference parcel size with NAPR ownership

---

### 3. SS.ge 🟡 OFFICIAL API EXISTS

**Website:** `https://home.ss.ge/ka/udzravi-qoneba/l/mitsis-nakveti/iyideba`
**API Info:** `https://ss.ge/ka/blog/ss-ge-api--186`
**Contact:** info@ss.ge, services.ss.ge, +995 322 121 661

**Platform stats:** ~40,000 daily unique users, ~550,000 monthly

**API capabilities (announced):**
- Post/manage listings
- Agent management
- Invoice requests
- Likely: search/filter listings (typical for marketplace APIs)

**Technical:** JavaScript SPA (React + styled-components)

**Data fields visible on site:**
- Price (GEL/USD)
- Area (m²)
- Location + address
- Property type
- Images
- View count

**Action required:**
- [ ] **Contact SS.ge for API access and documentation** (ESCALATE to Vadim — external contact)
- [ ] If no API read access → build scraper using headless browser (Playwright)

---

### 4. Myhome.ge 🔴 ANTI-BOT PROTECTION

**Website:** `https://www.myhome.ge` (part of MY.GE group, same as mymarket.ge)
**Status:** Returns 403 on programmatic access

⚠️ **mymarket.ge ≠ real estate.** Mymarket is general e-commerce. Myhome.ge is the real estate platform.

**Action required:**
- [ ] Research if myhome.ge has a public API
- [ ] If no API → headless browser with residential proxy (complex, P1)
- [ ] Update CLAUDE.md: replace "Mymarket.ge" with "Myhome.ge"

---

### 5. Place.ge 🟡 SCRAPABLE SPA

**Website:** `https://place.ge/en/ads?object_type=land`
**Stats:** ~2,132 total property ads

**URL patterns:**
- Base: `/en/ads?object_type=land`
- Batumi: `?city_id=3`
- Gonio: `?city_id=9`
- Kutaisi: `?city_id=66`

**Technical:** JavaScript SPA with client-side filtering

**Data fields:**
- Location (hierarchical: region → city → district → street)
- Object type
- Status (sale/lease)
- 130+ Georgian cities indexed

**Action required:**
- [ ] Build scraper (Playwright or API reverse-engineering)
- [ ] Map city_id values for Adjara coast

---

### 6. Copernicus Sentinel-2 🟢 FREE

**Portal:** `https://browser.dataspace.copernicus.eu/`
**API:** OpenEO Python client
**Resolution:** 10m multispectral
**Cost:** Free
**Coverage:** Global, including Georgia/Adjara

**Use cases for Plot Bot:**
- Vegetation analysis (greenery around plots)
- Beach/coastline monitoring
- Construction activity detection (temporal comparison)
- Terrain classification (rocky vs sandy vs forested)

**Action required:**
- [ ] Create Copernicus account
- [ ] Download sample imagery for target area (Grigoleti/Adjara coast)
- [ ] Build script: coordinates → satellite image → terrain classification

---

### 7. Public Transaction Prices 🔴 NOT FOUND

NAPR records transactions but does **not expose prices**. No public API for sale prices found.

**Possible alternatives:**
- SS.ge/Myhome listing prices (asking prices, not transaction prices)
- Contact NAPR directly about data access
- Research `napr.gov.ge/en` extract service (paid, per-property)
- Check if notary offices publish aggregated data

**Action required:**
- [ ] Research NAPR extract API (paid service with more data?)
- [ ] Ask Vadim's legal contacts about price data availability

---

### 8. Zoning & Land Use 🔴 NOT FOUND

No publicly accessible zoning map for Adjara found online. Georgian zoning is managed by municipal authorities.

**Possible sources:**
- Batumi Municipality Planning Department
- Georgian Ministry of Economy and Sustainable Development
- matsne.gov.ge — legislation on construction coefficients
- Local masterplans (may be physical documents only)

**Action required:**
- [ ] Research Batumi municipality urban planning portal
- [ ] Check matsne.gov.ge for construction/zoning regulations
- [ ] Ask Vadim's contacts about local zoning data access

---

## Additional Sources Discovered

| Source | URL | Type | Notes |
|--------|-----|------|-------|
| Livo.ge | livo.ge | Marketplace | New platform, real estate focus |
| AllHome.ge | allhome.ge | Marketplace | Real estate aggregator |
| Home.ge | home.ge | Marketplace | Real estate portal |
| MyBrokers.ge | mybrokers.ge | Marketplace | Broker-focused |
| Korter.ge | korter.ge | New developments | New construction focus |
| Realting.com | realting.com/georgia/lands | International | English-language, land listings |
| NoAgent.ge | noagent.ge | Guide | How-to for cadastral lookups |

---

## Recommended Priority

### Immediate (this session)
1. **Update CLAUDE.md** — fix mymarket.ge → myhome.ge
2. **SS.ge API** — prepare escalation to Vadim for contact

### Next session
3. **Place.ge scraper** — lowest barrier, 2K+ listings
4. **ArcGIS spatial queries** — enumerate all parcels in target area
5. **Copernicus setup** — satellite imagery pipeline

### Later
6. **SS.ge scraper/API integration** — after API access secured
7. **Myhome.ge** — after anti-bot research
8. **Transaction prices** — requires legal/government contacts

---

## Decision Required (ESCALATION)

**For Vadim:**
1. Contact SS.ge for API access? (info@ss.ge, services.ss.ge)
2. Any existing contacts at NAPR for price data?
3. Any contacts at Batumi municipality for zoning maps?
4. Confirm target area for MVP (Grigoleti? Other?)
