---
id: cross-provider-resilience
title: Provider Resilience
type: pattern
concern: [resilience]
mechanism: [state-machine, registry]
scope: per-item
lifecycle: [detect, act]
origin: harvest/voic-experiment
origin_findings: [2, 8, 14, 36, 43]
maturity: draft
domain: cross-agent/patterns
confidence: observed
basis: "distilled from voic-experiment harvest, 65 findings from voice agent sessions"
---

# Provider Resilience

<!-- CORE: load always -->
## Problem

External provider dependencies create single points of failure in agent pipelines. When a primary API provider goes down, the pipeline halts entirely -- even when alternative providers with identical capabilities are available. The agent has no mechanism to detect degradation, route around failures, or recover automatically.

Naive retry strategies amplify the problem. Retrying a failing provider with fixed intervals hammers an already-stressed service, worsening the outage for all consumers. Without health tracking, the agent cannot distinguish between a transient error (worth retrying) and a sustained outage (worth skipping). It continues sending requests to a broken provider until a human intervenes, wasting time and budget on requests that will never succeed.

Provider failures often correlate within a vendor: when one model from a vendor goes down, other models from the same vendor frequently fail simultaneously because they share infrastructure. Without awareness of this grouping, the fallback chain wastes requests trying sibling models that are equally unavailable.

## Solution

A provider health registry tracks the state of every configured provider using a per-provider state machine with three states: HEALTHY, DEGRADED, and COOLDOWN. Each successful request resets the provider to HEALTHY. Consecutive errors transition it through DEGRADED to COOLDOWN, where it is temporarily skipped by the fallback chain.

Cooldown durations follow exponential backoff with a configurable cap, preventing both premature retry (too aggressive) and permanent exclusion (too conservative). When a cooldown expires, the provider receives a single test request; success restores it to HEALTHY, failure doubles the cooldown.

An ordered fallback chain defines the priority sequence: primary, secondary, tertiary, and last-resort providers. The chain resolver iterates in order, skipping providers in cooldown. Prefix-aware group skipping extends this: if multiple providers share a vendor group prefix and that group shows failures, all providers in the group are deprioritized simultaneously, avoiding wasted requests on sibling models with shared infrastructure.

The entire provider configuration -- endpoints, authentication, pricing, rate limits, capabilities, and fallback order -- lives in a registry file (YAML or JSON). Adding, removing, or reordering providers requires only a config change, not a code deployment. Error records include the pipeline stage that triggered the failure, enabling post-hoc analysis of whether failures are provider-wide or integration-specific.

## Implementation

### Structure

```
PROVIDER_REGISTRY
├── providers: list[PROVIDER_CONFIG]
├── health: dict[provider_id → HEALTH_STATE]
└── fallback_chain: list[provider_id]    # ordered: primary → secondary → ... → fallback

PROVIDER_CONFIG
├── id: string                            # unique identifier
├── group: string                         # provider group prefix (e.g., vendor name)
├── endpoint: string                      # API endpoint URL
├── auth: AUTH_CONFIG                     # authentication details
├── pricing: PRICING_CONFIG               # cost per unit (for budget awareness)
├── rate_limits: RATE_LIMIT_CONFIG        # max requests per interval
└── capabilities: list[string]            # what this provider supports

HEALTH_STATE
├── status: HEALTHY | DEGRADED | COOLDOWN
├── consecutive_errors: int               # resets to 0 on success
├── cooldown_until: datetime | null       # null when healthy
├── last_success: datetime | null
├── last_failure: datetime | null
└── last_error: string | null             # most recent error message
```

### Health State Machine

```
HEALTHY → DEGRADED → COOLDOWN → HEALTHY
```

| Transition | Trigger | Action |
|------------|---------|--------|
| HEALTHY → DEGRADED | First error | consecutive_errors = 1, log warning |
| DEGRADED → DEGRADED | Subsequent error (< `{MAX_CONSECUTIVE_ERRORS}`) | consecutive_errors += 1 |
| DEGRADED → COOLDOWN | consecutive_errors >= `{MAX_CONSECUTIVE_ERRORS}` | Set cooldown_until, skip provider |
| COOLDOWN → HEALTHY | cooldown_until expired + next request succeeds | consecutive_errors = 0 |
| COOLDOWN → COOLDOWN | cooldown_until expired + next request fails | Double cooldown duration |
| Any → HEALTHY | Successful request | consecutive_errors = 0, cooldown_until = null |

### Exponential Backoff for Cooldown

```
cooldown_duration = min(
    {BASE_COOLDOWN} × 2^(consecutive_errors - {MAX_CONSECUTIVE_ERRORS}),
    {MAX_COOLDOWN}
)
```

| Consecutive Errors | Cooldown Duration |
|-------------------|-------------------|
| `{MAX_CONSECUTIVE_ERRORS}` | `{BASE_COOLDOWN}` (e.g., 30s) |
| `{MAX_CONSECUTIVE_ERRORS}` + 1 | `{BASE_COOLDOWN}` × 2 (e.g., 60s) |
| `{MAX_CONSECUTIVE_ERRORS}` + 2 | `{BASE_COOLDOWN}` × 4 (e.g., 120s) |
| `{MAX_CONSECUTIVE_ERRORS}` + N | Capped at `{MAX_COOLDOWN}` (e.g., 300s) |

### Fallback Chain Resolution

```
function select_provider(fallback_chain, health):
    for provider_id in fallback_chain:
        state = health[provider_id]
        if state.status == COOLDOWN and now() < state.cooldown_until:
            continue                          # skip: in cooldown
        if state.group in cooling_down_groups: # prefix-aware skip
            continue                          # skip: entire group is failing
        return provider_id
    raise AllProvidersExhausted()              # no provider available
```

Key behavior: **prefix-aware skipping**. If provider `{VENDOR_A}/model-1` enters cooldown and `{VENDOR_A}/model-2` also has errors, the entire `{VENDOR_A}/*` group is deprioritized. This prevents wasting requests on a vendor whose infrastructure is likely down.

### Error Propagation with Stage Context

Every error is logged with the pipeline stage that triggered it:

```
ERROR_RECORD
├── timestamp: datetime
├── provider_id: string
├── stage: string              # which pipeline stage was executing
├── error_type: string         # timeout | rate_limit | auth | server_error | invalid_response
├── error_message: string
├── request_context: object    # relevant request metadata (no PII)
└── fallback_to: string | null # which provider was tried next
```

This enables post-hoc analysis: "Provider X fails mostly at stage Y" reveals integration-specific issues vs. provider-wide outages.

### Registry Configuration

Provider configs are stored in a centralized registry file (YAML or JSON):

```yaml
providers:
  - id: "{PROVIDER_PRIMARY}"
    group: "{VENDOR_A}"
    endpoint: "{ENDPOINT_URL}"
    auth:
      type: api_key
      env_var: "{API_KEY_ENV_VAR}"
    pricing:
      unit: "{PRICING_UNIT}"
      cost_per_unit: "{COST}"
    rate_limits:
      requests_per_minute: "{RPM}"
    capabilities: ["{CAPABILITY_1}", "{CAPABILITY_2}"]

  - id: "{PROVIDER_SECONDARY}"
    group: "{VENDOR_B}"
    # ... same structure

fallback_chain:
  - "{PROVIDER_PRIMARY}"
  - "{PROVIDER_SECONDARY}"
  - "{PROVIDER_TERTIARY}"
  - "{PROVIDER_FALLBACK}"
```

Hot-swapping: reorder `fallback_chain`, add/remove providers, adjust rate limits — all via config. No code changes required.

### Decision Rules

| Situation | Action |
|-----------|--------|
| Primary provider succeeds | Use result, reset health |
| Primary provider fails once | Log warning, try next in chain |
| Provider hits `{MAX_CONSECUTIVE_ERRORS}` | Enter COOLDOWN, skip for `{BASE_COOLDOWN}` |
| All providers in cooldown | Raise `AllProvidersExhausted`, propagate error to caller |
| Provider returns after cooldown | Route one test request, if success → HEALTHY |
| Entire vendor group failing | Skip all providers with that group prefix |
| New provider added to registry | Append to fallback_chain, starts HEALTHY |

### Configuration

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `{MAX_CONSECUTIVE_ERRORS}` | Errors before cooldown | 3 | yes |
| `{BASE_COOLDOWN}` | Initial cooldown duration | 30s | yes |
| `{MAX_COOLDOWN}` | Maximum cooldown duration (cap) | 300s | yes |
| `{VENDOR_A}` | Primary vendor group name | "vendor-alpha" | yes |
| `{VENDOR_B}` | Secondary vendor group name | "vendor-beta" | yes |
| `{PROVIDER_PRIMARY}` | Primary provider ID | "vendor-alpha/model-fast" | yes |
| `{PROVIDER_SECONDARY}` | Secondary provider ID | "vendor-beta/model-standard" | yes |
| `{PROVIDER_TERTIARY}` | Tertiary provider ID | "vendor-gamma/model-balanced" | no |
| `{PROVIDER_FALLBACK}` | Last-resort provider ID | "vendor-alpha/model-legacy" | no |
| `{PRICING_UNIT}` | Unit for cost tracking | "1k tokens" | no |

<!-- REFERENCE: load on adoption -->
## Adaptation Guide

### Prerequisites
- Multiple providers available for the same capability (at least 2)
- A request/response interface to external services (HTTP clients, SDK wrappers)
- Centralized configuration mechanism (YAML/JSON config files or environment)
- Logging infrastructure for error records

### Steps to Adopt
1. Define the provider registry schema — list all available providers with their endpoints, auth, and capabilities
2. Implement the `HEALTH_STATE` object per provider (consecutive_errors, cooldown_until, status)
3. Implement the health state machine transitions (HEALTHY → DEGRADED → COOLDOWN → HEALTHY)
4. Implement exponential backoff cooldown calculation with cap
5. Implement the fallback chain resolver with prefix-aware group skipping
6. Wrap the existing service call in a resilient caller that iterates the fallback chain
7. Add error propagation with stage context logging
8. Store provider configs in a registry file (not hardcoded)
9. Add a health status endpoint or CLI command for observability
10. Test with simulated provider failures (chaos testing)

### What to Customize
- Number of providers and fallback chain order (depends on your vendor contracts)
- Cooldown timing (`{BASE_COOLDOWN}`, `{MAX_COOLDOWN}`) — tune to provider SLAs
- Error threshold (`{MAX_CONSECUTIVE_ERRORS}`) — lower = faster failover, higher = more tolerance for transient errors
- Group prefix logic — depends on whether your providers share infrastructure
- Capabilities list — match to your domain's requirements
- Pricing and rate limit tracking — optional but valuable for cost optimization

### What NOT to Change
- Per-provider health tracking — without it, failures are invisible until all requests fail
- Exponential backoff with cap — linear or no backoff either hammers failing providers or waits too long
- Success resets error counter — without this, a recovered provider stays in cooldown unnecessarily
- Ordered fallback chain — random selection loses the primary/secondary/fallback hierarchy
- Prefix-aware group skipping — without this, the system wastes requests on sibling providers of a failing vendor
- Config-driven registry — hardcoding providers forces code changes for every swap

<!-- HISTORY: load for audit -->
## Origin
- **Source agent:** voic-experiment
- **Findings:** [2] Provider health tracking with exponential backoff, [8] Ordered fallback chain with prefix-aware skipping, [14] Error propagation with stage context, [36] Provider diversity principle and registry pattern, [43] Hot-swapping via config
- **Discovered through:** Building a multi-provider pipeline where individual providers had unpredictable failure patterns. Initial implementation retried the same provider, causing cascading timeouts. Health tracking with cooldown was added to avoid retrying known-broken providers. Prefix-aware skipping was added after discovering that when one model from a vendor fails, other models from the same vendor often fail simultaneously (shared infrastructure). The registry pattern emerged from the need to swap providers without redeploying.
- **Evidence:** 30+ providers managed (7 LLM, 3 TTS, 2 STT). Fallback chain eliminated user-facing errors during single-provider outages. Exponential cooldown reduced wasted requests by an order of magnitude. Provider diversity achieved 10x cost reduction vs single-vendor lock-in. Config-driven registry enabled swaps in seconds instead of deploy cycles.

## Related Patterns
- [Validation Gates](../patterns/validation_gates.md) — pre-request gates can check provider health before attempting a call
- [Autonomous Loop Protocol](../patterns/loop_protocol.md) — provider health can feed into loop self-metrics
- [Monitoring Principles](../best_practices/monitoring_principles.md) — provider health is a key observability signal
- [Scenario-First Testing](../methodologies/scenario_first_testing.md) — test infrastructure benefits from resilient provider access
- [Adversarial Reflection](../patterns/adversarial_reflection.md) -- different providers for analyst/reviewer roles amplify adversarial effect
- [Context Budget Management](../methodologies/context_budget.md) -- fallback chains add context cost; budget accordingly
