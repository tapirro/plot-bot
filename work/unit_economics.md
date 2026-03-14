---
id: task-0.1-unit-economics
type: analysis
title: "Unit Economics — Model Plot (20,000 m²)"
domain: plot-bot/finance
status: draft
created: 2026-03-14
parent_bet: bet-plot-bot-roadmap
phase: "0.1"
---

# Unit Economics — Model Plot

Модельный расчёт на участок 20,000 м² в зоне Ureki/Grigoleti по $25/м².

## Параметры модели

| Parameter | Value | Source | Confidence |
|-----------|-------|--------|------------|
| Land buy price | $25/m² | SS.ge listings Ureki/Grigoleti | High |
| Total land area | 20,000 m² | Vision doc target | — |
| Sellable plots | 25 × 800 m² | Vision doc | — |
| Common areas (roads, green) | 25% = 5,000 m² | Industry standard | Medium |
| Target sell price (raw plot) | $200/m² | Conservative vs $350 vision | Medium |
| Target sell price (plot + project) | $280/m² | With architectural project | Medium |
| Capital gains tax | 0% | Hold >2 years | High |
| Registration fee | $50 per plot | NAPR standard | High |
| Annual land tax | $0.08/m² | Georgian tax code | High |

---

## Phase 1: Acquisition

| Item | Calculation | Amount |
|------|-------------|--------|
| Land purchase | 20,000 m² × $25 | **$500,000** |
| Registration | 1 transaction × $50 | $50 |
| Due diligence (legal) | Est. | $2,000 |
| **Total acquisition** | | **$502,050** |

## Phase 2: Financing

Assuming bank credit for 70% of acquisition cost.

| Item | Calculation | Amount |
|------|-------------|--------|
| Equity required (30%) | $502K × 0.30 | **$150,615** |
| Bank credit (70%) | $502K × 0.70 | **$351,435** |
| Interest rate | 14% (commercial land) | — |
| Credit term | 24 months | — |
| Monthly payment | Annuity | ~$16,900/mo |
| Total interest (2 years) | | **$54,200** |

## Phase 3: Development (облагораживание)

| Item | Calculation | Amount |
|------|-------------|--------|
| Land survey + subdivision | 25 plots × $200 | $5,000 |
| Access road (gravel, 500m) | 500m × $50/m | $25,000 |
| Electricity connection | 25 plots × $400 | $10,000 |
| Water connection | 25 plots × $800 | $20,000 |
| Perimeter greenery | 2,000m × $15/m | $30,000 |
| Landscaping common areas | 5,000 m² × $5/m² | $25,000 |
| Architectural standards (Архнадзор) | Setup + guidelines | $10,000 |
| Marketing (landing, photos, drone) | | $5,000 |
| Management/overhead (12 months) | | $12,000 |
| **Total development** | | **$142,000** |

## Phase 4: Sales

### Scenario A: Sell raw plots at $200/m²

| Item | Calculation | Amount |
|------|-------------|--------|
| Revenue (25 plots × 800 m²) | 20,000 m² × $200 | **$4,000,000** |
| Less: non-sellable common areas | -5,000 m² | — |
| **Adjusted revenue** | 15,000 m² × $200 | **$3,000,000** |
| Registration fees (25 sales) | 25 × $50 | $1,250 |
| Agent commissions (3%) | $3M × 0.03 | $90,000 |
| Capital gains tax | 0% (>2 years) | $0 |

### Scenario B: Sell plot + architectural project at $280/m²

| Item | Calculation | Amount |
|------|-------------|--------|
| **Adjusted revenue** | 15,000 m² × $280 | **$4,200,000** |
| Architectural projects (25) | 25 × $3,000 | $75,000 |
| Registration + commissions | | $127,250 |

---

## P&L Summary

| Line | Scenario A ($200/m²) | Scenario B ($280/m²) |
|------|---------------------|---------------------|
| **Revenue** | $3,000,000 | $4,200,000 |
| Land acquisition | -$502,050 | -$502,050 |
| Interest expense | -$54,200 | -$54,200 |
| Development costs | -$142,000 | -$142,000 |
| Architectural projects | $0 | -$75,000 |
| Sales costs | -$91,250 | -$127,250 |
| **Total costs** | **-$789,500** | **-$900,500** |
| **Net profit** | **$2,210,500** | **$3,299,500** |
| **ROI** | **3.8x** | **4.7x** |
| ROI on equity only | **14.7x** | **21.9x** |

---

## Break-Even Analysis

| Metric | Scenario A | Scenario B |
|--------|-----------|-----------|
| Total costs to recover | $789,500 | $900,500 |
| Revenue per plot (800 m²) | $160,000 | $224,000 |
| **Plots to break even** | **5 of 25** | **4 of 25** |
| Break-even at % of inventory | 20% | 16% |

**Key insight:** Need to sell only 4-5 plots (16-20% of inventory) to cover ALL costs including land, credit, and development.

---

## Cash Flow Timeline (24 months)

| Month | Event | Cash In | Cash Out | Balance |
|-------|-------|---------|----------|---------|
| 0 | Equity investment | +$150,615 | | $150,615 |
| 0 | Bank credit | +$351,435 | | $502,050 |
| 0 | Land purchase | | -$502,050 | $0 |
| 1-6 | Development | | -$142,000 | -$142,000 |
| 1-6 | Credit payments | | -$101,400 | -$243,400 |
| 6 | First sales begin | | | |
| 6-12 | Sell 5 plots (break-even) | +$800,000 | | $556,600 |
| 7-12 | Credit payments | | -$101,400 | $455,200 |
| 12 | Credit fully repaid | | | $455,200 |
| 12-24 | Sell remaining 20 plots | +$2,200,000 | | $2,655,200 |
| 24 | Final sales costs | | -$91,250 | **$2,210,500** |

⚠️ **Cash balance goes negative months 1-6** — violates invariant #3 (positive cash balance always).

### Fix: Phased approach
- Pre-sell 2-3 plots at month 0-1 at discount ($150/m² = $360,000)
- Or: require 40% equity ($200K) instead of 30%
- Or: negotiate credit disbursement in tranches tied to development milestones

---

## Sensitivity Analysis

| Variable | Change | Impact on Net Profit |
|----------|--------|---------------------|
| Buy price +$10/m² | $25→$35 | -$200K (-9%) |
| Sell price -$50/m² | $200→$150 | -$750K (-34%) |
| Sell price +$50/m² | $200→$250 | +$750K (+34%) |
| Interest rate +4% | 14%→18% | -$30K (-1.4%) |
| Development costs +50% | $142K→$213K | -$71K (-3.2%) |
| Only sell 15 of 25 plots | 60% sold | Revenue drops to $1.8M, still profitable |

**Most sensitive to:** sell price. $150/m² still profitable ($1.46M net). Below $53/m² = loss.

---

## Key Risks

1. **Sell price uncertainty** — $200/m² is assumption, no comparable sales data for developed plots in Grigoleti
2. **Timeline** — assumes 18-24 months full cycle, could be longer
3. **Infrastructure costs** — low confidence on water/electricity/road costs
4. **Demand risk** — who buys $160K plots in Grigoleti? Need buyer persona
5. **Cash flow gap** — months 1-6 require bridge financing or pre-sales
6. **Regulatory** — agricultural land restrictions for foreigners (research needed)

---

## Decisions Required (ESCALATION)

1. **Equity vs debt ratio** — 30/70 creates cash gap. 40/60 or 50/50 safer?
2. **Target buyer** — who exactly pays $200/m² for a plot in Grigoleti?
3. **Pre-sale model** — sell plots before development is complete?
4. **Kapravi comparable** — what exactly is the Kapravi pricing? ($350/m² mentioned)
5. **Minimum cluster size** — 20,000 m² or start smaller (5,000 m²)?

---

## Comparison with Vision Target

| Metric | Vision Doc | This Model | Gap |
|--------|-----------|------------|-----|
| Buy price | $20-30/m² | $25/m² ✅ | On target |
| Sell price | $200-350/m² | $200/m² (conservative) | Could be higher |
| ROI | 10x | 3.8x (total) / 14.7x (equity) | **Equity ROI exceeds 10x** |
| Cluster | 20,000 m² → 800 m² plots | 25 plots ✅ | On target |

**Conclusion:** 10x ROI achievable on equity (14.7x). Total capital ROI is 3.8x due to development costs. The leverage effect is the key — 70% debt financing amplifies equity returns.
