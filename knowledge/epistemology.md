---
id: epistemology
type: methodology
title: "Epistemological Framework — Data Trust & Confidence"
domain: plot-bot
status: active
created: 2026-03-14
basis: "adapted from cross-agent confidence_severity model + assistant epistemology + real estate domain requirements"
confidence: validated
origin: "cross-agent confidence_severity model + real estate domain requirements"
---

# Epistemological Framework

## problem: Problem

Real estate land acquisition involves real money decisions based on data from multiple sources of varying reliability (official registries, marketplace listings, model outputs, anecdotal reports).
Without a systematic framework for tracking data provenance, confidence, and freshness, the agent risks making purchase recommendations based on stale, unverified, or conflicting information — leading to financial loss.
Key failure modes: presenting model output as fact, using expired price data, silently resolving source conflicts, and making recommendations without sufficient verification depth.

## solution: Solution

A layered epistemological framework with four pillars:
1. **Source Trust Hierarchy** (L1-L7) — assigns base confidence by source type
2. **Data Classification** — categorizes facts by freshness SLA and decay rate
3. **Decision Gates** — blocks actions when confidence falls below thresholds
4. **Provenance Tracking** — enforces `[C:X.XX S:<source> V:<date>]` on every numeric fact

The framework includes conflict resolution protocols, correction cascades, and anti-patterns that prevent the most dangerous epistemic failures (presenting model output as observed fact, omitting provenance from financial data, using stale prices without marking).

This domain deals with real money decisions. Every piece of data has a trust level, a source, a freshness window, and a verification status. This framework defines how Plot Bot classifies, tracks, and gates decisions on data confidence.

## Core Principle

**No data point is trustworthy by default.** Every fact must carry its provenance, confidence, and freshness. Decisions are gated on confidence thresholds — not on data availability alone.

## 1. Source Trust Hierarchy

| Level | Source | Trust | Example |
|-------|--------|-------|---------|
| **L1** | Vadim's direct input | 1.0 (canonical) | "Кластерный подход — только Григолети" |
| **L2** | Official registries (NAPR, ArcGIS) | 0.85 | Cadastral area, ownership, encumbrances |
| **L3** | Cross-referenced marketplace data | 0.70 | Price from Place.ge confirmed by SS.ge |
| **L4** | Single marketplace listing | 0.45 | One listing on Place.ge |
| **L5** | Own analysis/modeling | 0.40 | Computed score from scoring model |
| **L6** | Gemini research output | 0.30 | Market trend summary from web search |
| **L7** | Unverified / anecdotal | 0.15 | "Heard that land near X is cheap" |

**Rule:** When sources conflict, higher trust level wins. When same-level sources conflict, flag as `⚠️ CONFLICT` and escalate.

## 2. Data Classification

Every data point belongs to one of these classes:

| Class | Description | Freshness SLA | Decay Rate | Examples |
|-------|-------------|---------------|------------|---------|
| **MARKET** | Prices, listings, availability | 7 days | -0.10/week | Listing price, asking price/m² |
| **CADASTRAL** | Registry data, ownership, encumbrances | 30 days | -0.05/month | NAPR ownership, mortgage status |
| **GEOGRAPHIC** | Physical properties of land | 1 year | -0.02/year | Area, slope, distance to sea |
| **LEGAL** | Laws, regulations, tax rules | 6 months | -0.05/6mo | Foreign ownership rules, tax rates |
| **FINANCIAL** | Cost estimates, projections | 30 days | -0.10/month | Construction costs, ROI projections |
| **STRATEGIC** | Operator decisions, invariants | Indefinite | 0 | Cluster approach, min ROI target |

**Staleness rule:** When data exceeds its SLA, mark `⚠️ STALE (since YYYY-MM-DD)` and reduce confidence by decay rate.

## 3. Confidence Scoring

Every fact in a deliverable MUST carry a confidence annotation:

```
Format: [confidence: X.XX, source: <source>, verified: <date>]
  or shorthand: [C:0.70 S:place.ge V:2026-03-14]
```

### Initial Confidence Assignment

| Verification Mode | Base Confidence | When |
|-------------------|-----------------|------|
| **Verified** (site visit + documents) | 0.85 | Before any purchase decision |
| **Enriched** (multiple remote sources) | 0.60 | After cross-referencing 2+ sources |
| **Remote** (single source) | 0.35 | First-pass analysis only |
| **Estimated** (model output) | 0.30 | Scoring model, projections |
| **Unverified** (no source) | 0.10 | Placeholder, must be replaced |

### Confidence Modifiers

| Event | Effect |
|-------|--------|
| Cross-reference confirms (2nd source) | +0.15 |
| Cross-reference confirms (3rd source) | +0.10 |
| Official registry confirms | +0.20 |
| Site visit confirms | +0.25 |
| Data exceeds freshness SLA | -0.10 per SLA period |
| Source known to have errors | -0.15 |
| Vadim explicitly validates | → 1.0 |

### Confidence Lifecycle

```
ACQUIRE → assign initial confidence from verification mode
  → CROSS-REF → check against other sources → adjust confidence
  → AGE → apply staleness decay per data class
  → VALIDATE → site visit or official confirmation → upgrade
  → CONFLICT → sources disagree → flag, escalate, investigate
  → EXPIRE → confidence < 0.20 → mark DEAD, must re-acquire
```

## 4. Decision Confidence Gates

**No action without sufficient confidence:**

| Decision Type | Min Confidence | Escalation |
|---------------|---------------|------------|
| **Purchase recommendation** | 0.85 (Verified) | Always escalate to Vadim |
| **Financial projection** | 0.60 (Enriched) | Include confidence range |
| **Scoring / ranking** | 0.35 (Remote) | May proceed autonomously |
| **Research direction** | 0.15 (any) | May proceed autonomously |
| **Publish to investor** | 0.85 (Verified) | Always escalate to Vadim |

**Hard rule:** If confidence for a purchase recommendation < 0.85, the cycle MUST produce an investigation task instead of a recommendation. Escalating low-confidence data as if it were high-confidence = epistemological violation → immediate stop.

## 5. Provenance Tracking

### In Artifacts (markdown)

Every numeric fact in `work/` deliverables must include provenance:

```markdown
<!-- CORRECT -->
Средняя цена в Григолети: **$28/м²** [C:0.70 S:place.ge+arcgis V:2026-03-14, N=28 listings]

<!-- WRONG -->
Средняя цена в Григолети: $28/м²
```

### In Database (SQLite)

Every record must carry:
- `source` — origin system (place.ge, napr, arcgis, manual)
- `scraped_at` — when data was acquired
- `confidence` — current confidence score (0.0–1.0)
- `verified_at` — last verification date (NULL if never verified)
- `stale` — boolean, auto-set when `scraped_at` exceeds SLA

### In Cycle Reports

Every cycle report MUST include in `## Changes`:
```
- Created `work/X.md` [verified: 2 sources, C:0.60]
- Updated `work/Y.md` [unverified: single source, C:0.35, needs cross-ref]
```

## 6. Conflict Resolution Protocol

When two sources provide different values for the same fact:

1. **Log both values** with their sources and confidence
2. **Flag as `⚠️ CONFLICT`** in the artifact
3. **Prefer higher-trust source** (see §1 hierarchy)
4. **If same trust level:** investigate (check timestamps, methodology, coverage)
5. **If unresolvable:** escalate to Vadim with both values + analysis
6. **Never silently pick one** — the conflict itself is information

## 7. Correction Protocol

When a previously published fact is found to be wrong:

1. Mark original with `⚠️ CORRECTION PENDING` (first line after frontmatter)
2. Create P0 fix task in `context/cycle_plan.md`
3. Log correction in cycle report: old value → new value, reason, impact
4. Update all dependent artifacts (cascade check)
5. If correction affects a score: recalculate + note `[recalculated: YYYY-MM-DD, reason]`
6. If correction affects a financial projection: escalate to Vadim

## 8. Anti-Patterns (NEVER)

- **NEVER** present L5-L7 data with L1-L2 confidence framing
- **NEVER** omit provenance from financial or price data
- **NEVER** mix verified and unverified in the same table column without marking
- **NEVER** use a stale price (>7 days) without `⚠️ STALE` marker
- **NEVER** recommend a purchase based on Remote-mode data alone
- **NEVER** skip the cross-reference step for any price or area figure
- **NEVER** present model output as observed fact
