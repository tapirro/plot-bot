---
id: cross-data-quality
title: Data Quality Framework
type: methodology
concern: [data-integrity]
mechanism: [decision-tree, registry]
scope: per-item
lifecycle: [detect, classify]
origin: harvest/hilart-ops-bot
origin_findings: [3]
maturity: draft
domain: cross-agent/methodologies
confidence: observed
basis: "distilled from hilart-ops-bot harvest, 42 findings from ops bot sessions"
---

# Data Quality Framework

<!-- CORE: load always -->
## Problem

When a data pipeline goes down, downstream metrics drop to zero or show extreme deviations. To the monitoring agent, this looks indistinguishable from a genuine business collapse -- recovered leads drop 60% across all regions, order counts flatline, conversion rates crater. Without a data quality pre-check, the agent classifies these as critical business incidents and escalates them as crises.

The cost is twofold. The immediate cost is wasted investigation effort: the agent spends cycles diagnosing a business problem that does not exist, querying stakeholders and analyzing trends that are artifacts of missing data rather than real market movements. The downstream cost is credibility erosion: repeated false crisis escalations train human operators to ignore alerts, so when a real business issue occurs, it gets the same dismissive response.

Pipeline outages are not rare edge cases -- they are a regular operational reality. CDC pipelines stall, ETL jobs fail silently, API rate limits get hit, and data sources go through maintenance windows. Each of these events produces metric anomalies that look like business problems unless the agent checks data quality first.

## Solution

A mandatory four-step decision tree is executed before classifying any anomaly as a business issue. The tree checks data quality in order of likelihood and cost: first, is the data source fresh (comparing last update time against a per-source freshness SLA)? Second, are multiple tables from the same source affected with the same cutoff time (indicating a pipeline issue rather than a business anomaly)? Third, does the anomaly match a known data pattern from the living registry of documented gotchas? Only if all three checks pass does the anomaly proceed to business classification.

Data quality problems receive their own incident taxonomy (`DATA-STALE`, `DATA-DOWN`, `DATA-PIPELINE`, `DATA-GAP`), kept separate from business incidents. This separation ensures that infrastructure issues are routed to the right responders and do not pollute business metrics tracking.

Each data source has a defined freshness SLA with an expected lag duration and a health check mechanism (SQL query, API call, or file timestamp). The known data patterns registry is a living document fed by concluded investigations -- every time an investigation reveals a data artifact rather than a business issue, the pattern is documented for future automatic detection.

The key principle is ordering: data quality is always checked before business logic. This single architectural decision -- making the decision tree mandatory and first -- prevents the most common and most expensive class of false positives in operational monitoring.

## Implementation

### Structure

```
DATA_SOURCE
├── name: string
├── health_check_query: string
├── expected_lag: duration
├── last_check: timestamp
└── status: OK | STALE | DOWN
```

### Decision Tree

```
BEFORE classifying anomaly as business issue:

1. Is the data source fresh?
   → NO → Create DATA-STALE or DATA-DOWN incident
   → YES → Continue

2. Are multiple tables from the same source affected with the same cutoff time?
   → YES → Pipeline issue, not business anomaly
          Create DATA-PIPELINE incident
   → NO → Continue

3. Does the anomaly match a known data pattern?
   → YES → Apply correction factor, note in report
   → NO → Continue

4. All checks pass
   → Classify as business anomaly per incident taxonomy
```

### Configuration

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `{SOURCE_NAME}` | Name of data source | "CRM", "ERP" | yes |
| `{EXPECTED_LAG}` | Maximum acceptable data lag | 30min, 1h, 24h | yes |
| `{HEALTH_CHECK}` | Query/endpoint to check freshness | SQL, API call | yes |
| `{KNOWN_PATTERNS}` | Registry of known data gotchas | List of patterns | recommended |

### Pipeline Freshness SLA Table

| Source | Expected Lag | Health Check |
|--------|-------------|-------------|
| `{SOURCE_1}` | `{LAG_1}` | `{CHECK_1}` |
| `{SOURCE_2}` | `{LAG_2}` | `{CHECK_2}` |
| ... | ... | ... |

### Known Data Patterns Registry

Maintain a registry of documented data gotchas:

```
KNOWN_PATTERN
├── id: int
├── description: string
├── source_investigation: string (e.g., "INV-001")
├── impact: string
├── correction: string (how to handle it)
└── last_seen: date
```

New entries come from concluded investigations. This is a living document.

<!-- REFERENCE: load on adoption -->
## Adaptation Guide

### Prerequisites
- Access to data source health metrics (freshness, row counts)
- Incident taxonomy that includes DATA-* category

### Steps to Adopt
1. List all data sources the agent depends on
2. For each source, define expected lag and health check mechanism
3. Implement the 4-step decision tree as first step in anomaly detection
4. Create DATA-* incident types: DATA-STALE, DATA-DOWN, DATA-PIPELINE, DATA-GAP
5. Start a Known Data Patterns registry (empty is fine — it fills from investigations)
6. Run the decision tree BEFORE any business classification

### What to Customize
- Data sources and their SLAs
- Health check queries/endpoints
- Known patterns (grows organically from investigations)
- Specific DATA-* incident subtypes

### What NOT to Change
- The 4-step sequence (freshness → pipeline → known pattern → business)
- Data problems get their own taxonomy, not mixed with business
- Known Patterns as a living registry fed by investigations
- The principle: always check data quality BEFORE blaming business

<!-- HISTORY: load for audit -->
## Origin
- **Source agent:** hilart-ops-bot
- **Findings:** [3] Data Quality Decision Tree
- **Discovered through:** INV-002 (recovered leads -58-64% across all regions) was caused by a CDC pipeline outage, not a business collapse. Without the data quality pre-check, this would have been escalated as a business crisis. After adding the decision tree, similar pipeline outages were correctly classified as DATA-PIPELINE incidents.
- **Evidence:** INV-002: recovered leads -58-64% across all regions was CDC pipeline outage, not business collapse — decision tree prevented false crisis escalation. Known patterns registry grew to 7 entries from 3 investigations.

## Related Patterns
- [Metric Classification](../methodologies/metric_classification.md) — LAGGING suppression works with data quality checks
- [Confidence & Severity Model](../methodologies/confidence_severity.md) — pipeline DOWN reduces confidence ×0.5
- [Monitoring Principles](../best_practices/monitoring_principles.md) — semantic errors lesson
- [Incident Taxonomy](../patterns/incident_taxonomy.md) — DATA-* types come from data quality checks
- [Scenario-First Testing](../methodologies/scenario_first_testing.md) — input pre-generation is a data quality practice
