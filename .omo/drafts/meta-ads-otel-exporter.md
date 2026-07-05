---
slug: meta-ads-otel-exporter
status: approved
intent: clear
pending-action: write .omo/plans/meta-ads-otel-exporter.md
approach: Single Go binary daemon that polls Meta Marketing API for campaign-level insights and exports as OTel gauges to Mimir via OTLP/HTTP
---

# Draft: meta-ads-otel-exporter

## Components (topology ledger)
<!-- Lock the SHAPE before depth. One row per top-level component that can succeed or fail independently. -->
<!-- id | outcome (one line) | status: active|deferred | evidence path -->

| ID | Component | Outcome | Status | Evidence |
|----|-----------|---------|--------|----------|
| C1 | Meta API Integration | Fetch campaigns + today's insights (17 fields) from Meta Ads API | active | Meta Marketing API /insights endpoint |
| C2 | OpenTelemetry Export | Convert metrics to gauges, export via OTLP to Mimir | active | go.mod:10 (otel/metric v1.44.0) |
| C3 | Daemon Runtime | Poll scheduler (10min), graceful shutdown, health endpoint | active | cmd/main.go (empty, to be implemented) |
| C4 | Configuration System | Load YAML config + env var secrets, validate on startup | active | - |
| C5 | Local Dev Environment | Docker Compose with Mimir + exporter for testing | active | docker-compose.yaml (empty) |

## Open assumptions (announced defaults)
<!-- Record any default you adopt instead of asking, so the user can veto it at the gate. -->
<!-- assumption | adopted default | rationale | reversible? -->

| Assumption | Adopted Default | Rationale | Reversible? |
|------------|----------------|-----------|-------------|
| Poll interval exact value | 10 minutes | Middle of 5-15min range, safe rate-limit margin | Yes (config param) |
| OTLP protocol | HTTP | Simpler than gRPC, no proto compilation | Yes (exporter swap) |
| Health endpoint port | 8080 | Standard non-privileged port | Yes (config param) |
| Config file path | ./config.yaml with --config flag | Standard convention | Yes (flag) |
| Service name | meta-ads-exporter | Matches binary name | Yes (config param) |
| Metric naming | meta_ads.* prefix | Namespaced, matches source | Yes (code change) |
| Graceful shutdown timeout | 30 seconds | Standard for daemons | Yes (config param) |
| Mimir docker setup | Single-process, local storage, no auth | Dev environment, not production | Yes (docker-compose) |

## Findings (cited - path:lines)

- go.mod:3 - Go 1.25.0 project
- go.mod:10-13 - OpenTelemetry SDK v1.44.0 already present (otel, otel/metric, otel/sdk)
- cmd/main.go - Empty file, no existing implementation
- docker-compose.yaml - Empty file, needs Mimir + exporter services
- User has Meta access token already provisioned (long-lived)
- Single ad account, Standard Access tier (no elevated rate limits)
- Meta Marketing API reference: https://developers.facebook.com/docs/marketing-api/reference/adgroup/insights
- 17 fields confirmed: spend, account_currency, date_start, date_stop, impressions, clicks, ctr, cpm, cpc, reach, frequency, conversions, cost_per_conversion, conversion_rate, roas (plus campaign_id, campaign_name as labels)

## Decisions (with rationale)

1. **Test strategy: tests-after** - User confirmed. Implement components first, then unit tests for Meta API client, metric conversion, backoff logic. Agent-executed QA in all todos regardless.

2. **Campaign level only** - User confirmed. Out: ad set/ad level (higher cardinality, deferred to future).

3. **Poll "today" data every 10 minutes** - User confirmed 5-15min range, adopted 10min default (safe rate-limit margin, near-real-time for alerting). Note: "today" uses account timezone.

4. **Field classification (Metis-corrected)** - 17 fields are NOT all gauges:
   - **Gauges (9 simple)**: spend, impressions, clicks, ctr, cpm, cpc, reach, frequency, roas (see D8)
   - **Gauges (3 derived)**: conversions, conversion_rate, cost_per_conversion (see D8)
   - **Labels/attributes**: campaign_id, campaign_name, account_currency
   - **Metadata (labels or drop)**: date_start, date_stop (ISO dates, not numeric gauges)

5. **OpenTelemetry Gauges, not Counters** - Meta returns current totals for "today" which can be retroactively adjusted. Gauges handle this; Counters assume monotonic increase and would mis-report.

6. **OTLP/HTTP, not gRPC** - Simpler, no proto compilation, standard Mimir ingestion path. Need to add `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp` dependency (not in current go.mod).

7. **Rate-limit detection (Metis-corrected)** - Meta throttle errors are HTTP 200/400 with error codes (4, 17, 80000-80004, 613) in JSON body, NOT HTTP 429. Parse `{"error": {"code": N}}` and check `X-Business-Use-Case-Usage` header for `estimated_time_to_regain_access` (minutes). Exponential backoff if header absent.

8. **Nested action field extraction** - conversions/roas/cost_per_conversion are action arrays, not flat scalars:
   - `actions` returns `[{action_type: "purchase", value: "5"}, ...]`
   - Extract: sum values where `action_type` in ["purchase", "offsite_conversion.fb_pixel_purchase"]
   - `roas` field is actually `purchase_roas` array, extract similarly
   - `conversion_rate` is DERIVED: `conversions / clicks` (not a Meta field)
   - `cost_per_conversion` is DERIVED: `spend / conversions` (or may be `cost_per_action_type` array - verify)

9. **Empty result / dropout behavior**:
   - Campaign with no delivery today (empty `data` array): emit gauge value 0 for all numeric fields
   - Campaign disappeared from results: leave gauge stale (last value persists in Mimir), do not zero out
   
10. **OTLP export failure handling** - Retry with exponential backoff (separate from Meta-side backoff). Max 3 retries, then drop batch and log error. Do not block Meta polling on Mimir downtime.

11. **Token expiry** - On auth failure (Meta error code 190 or 102), log fatal error and exit non-zero. Requires human intervention (token rotation out of scope).

12. **Pagination** - Both campaigns list (`/act_{id}/campaigns`) and insights paginate. Follow `paging.next` cursors until exhausted.

13. **Docker Compose with Mimir** - User confirmed. Single-process Mimir for local dev, not production HA setup.

## Scope IN

- Meta Marketing API client fetching campaign-level insights for one ad account
- 17 fields: campaign_id, campaign_name, spend, account_currency, date_start, date_stop, impressions, clicks, ctr, cpm, cpc, reach, frequency, conversions, cost_per_conversion, conversion_rate, roas
- `date_preset=today`, polled every 10 minutes
- OpenTelemetry gauge metrics (meta_ads.* namespace) with campaign_id/campaign_name/account_id/currency attributes
- OTLP/HTTP export to Grafana Mimir
- YAML config (account ID, poll interval, OTLP endpoint) + env var secrets (META_ACCESS_TOKEN)
- JSON structured logging (slog), INFO default
- Exponential backoff + Meta rate-limit header handling
- /healthz and /readyz HTTP endpoints on port 8080
- Graceful shutdown (30s timeout)
- Docker Compose: Mimir (single-process, local storage, no auth) + exporter service
- Unit tests (tests-after) for Meta API client parsing, metric conversion, backoff logic

## Scope OUT (Must NOT have)

- Ad set or ad level metrics (campaign level only)
- Multiple ad accounts (single account only)
- Historical backfill beyond "today" (no date range beyond today's preset)
- Token refresh/rotation automation (token already provisioned as long-lived)
- Production Mimir HA/auth/multi-tenancy setup (dev-only docker-compose)
- Sub-minute polling (rate limits + data latency make this infeasible, confirmed with user)
- Counter-based metrics (gauges only, per decision 4)
- gRPC OTLP transport (HTTP only, per decision 5)
- Kubernetes manifests or orchestration beyond docker-compose
- Alerting rules or dashboards (Mimir/Grafana config is user's downstream responsibility)

## Open questions

None remaining - all surfaced forks resolved during grilling session and approval brief.

## Approval gate
status: approved
<!-- When exploration is exhausted and unknowns are answered, set status: awaiting-approval. -->
<!-- That durable record is the loop guard: on a later turn read it and resume at the gate instead of re-running exploration. -->

Brief presented and user approved on 2026-07-05 13:47Z. Proceeding to plan generation with mandatory Metis review before todo appending.
