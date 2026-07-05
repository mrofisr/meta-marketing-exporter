# meta-ads-otel-exporter - Work Plan

## TL;DR (For humans)
<!-- Fill this LAST, after the detailed plan below is written, so it summarizes the REAL plan. -->
<!-- Plain English for a non-engineer: NO file paths, NO todo numbers, NO wave/agent/tool names. -->

**What you'll get:** A single Go program that runs continuously, fetching your Meta ad campaign performance every 10 minutes and sending it to your Grafana Mimir monitoring system. You'll see metrics for spend, impressions, clicks, conversions, and return-on-ad-spend as live graphs you can alert on.

**Why this approach:** Meta's API returns some metrics as nested data structures rather than simple numbers, and their rate-limiting works differently than most APIs (error codes in the response body, not HTTP status). The plan accounts for both, plus handles pagination, empty results, and network failures gracefully so the system keeps running even when things go wrong.

**What it will NOT do:** Won't track individual ads or ad sets (only campaign-level summaries). Won't store historical data beyond "today" (no backfill). Won't auto-refresh your API token when it expires (you'll need to update it manually). The included Mimir setup is for local testing only, not production use.

**Effort:** Medium (9 implementation tasks across 4 waves, plus integration testing)

**Risk:** Medium - depends on correct Meta API field extraction (conversions/roas are nested arrays requiring special parsing) and proper rate-limit detection (must check error codes, not just HTTP status). Both are well-specified in the plan but require careful implementation.

**Decisions to sanity-check:** The plan assumes your Meta campaigns use standard conversion action types ("purchase", "offsite_conversion.fb_pixel_purchase"). If your account uses custom conversion events, you'll need to adjust the action_type filter in todo 6. Also, "today" is evaluated in your ad account's configured timezone, which affects the midnight rollover boundary.

Your next move: Review the 9 todos below, then start work via the worker agent or run a high-accuracy dual-Momus review first (your choice). Full execution detail follows below.

---

> TL;DR (machine): Medium effort, medium risk (Meta API shape + rate-limit semantics). Delivers: campaign-level Meta Ads metrics → OTel gauges → Mimir via OTLP/HTTP, 10min poll, docker-compose dev stack, graceful error handling.

## Scope
### Must have
- Meta Marketing API client for one ad account
- Campaign-level insights fetch with `date_preset=today`, 10-minute poll interval
- Field extraction with correct classification:
  - **Simple gauges (9)**: spend, impressions, clicks, ctr, cpm, cpc, reach, frequency, roas (from `purchase_roas` array)
  - **Derived gauges (3)**: conversions (sum from `actions` array), conversion_rate (conversions/clicks), cost_per_conversion (spend/conversions)
  - **Attributes**: campaign_id, campaign_name, account_currency, date_start, date_stop
- Nested action array extraction: sum values where `action_type` in ["purchase", "offsite_conversion.fb_pixel_purchase"]
- OpenTelemetry gauge metrics with `meta_ads_*` naming (note: Mimir transforms to `meta_ads_*` with underscores)
- OTLP/HTTP export to Grafana Mimir endpoint
- Rate-limit detection via Meta error codes (4, 17, 80000-80004, 613) in response body, not HTTP status
- Exponential backoff honoring `X-Business-Use-Case-Usage` header `estimated_time_to_regain_access`
- Pagination handling for campaigns list and insights
- Empty result behavior: emit 0 for numeric gauges when campaign has no delivery
- Campaign dropout behavior: leave gauges stale (last value persists)
- OTLP export failure: retry with backoff (max 3), then drop batch and log
- Token expiry: fatal error and exit non-zero (auth errors 190, 102)
- YAML config (account ID, poll interval, OTLP endpoint) + env var secrets (META_ACCESS_TOKEN)
- JSON structured logging (slog), INFO default
- /healthz (process alive) and /readyz (first successful poll completed) on port 8080
- Graceful shutdown (SIGTERM, 30s timeout)
- Docker Compose: Mimir (single-process, local storage) + exporter service
- Unit tests for Meta API parsing, metric conversion, backoff logic

### Must NOT have (guardrails, anti-slop, scope boundaries)
- Ad set or ad level metrics (campaign only)
- Multiple ad accounts (single account only)
- Historical backfill beyond "today"
- Token refresh/rotation automation
- Production Mimir HA/auth/multi-tenancy
- Sub-minute polling (rate limits prohibit)
- Counter-based metrics (gauges only)
- gRPC OTLP transport (HTTP only)
- Kubernetes manifests
- Alerting rules or Grafana dashboards (downstream user responsibility)
- HTTP 429-only rate-limit detection (must check error codes)
- Flat-scalar assumption for conversions/roas/cost_per_conversion (nested arrays require extraction)

## Verification strategy
> Zero human intervention - all verification is agent-executed.
- Test decision: **tests-after** + Go standard `testing` package (table-driven tests)
- Framework: `go test ./...` for unit tests; `docker compose` + `curl`/`grpcurl` for integration QA
- Evidence: .omo/evidence/task-<N>-meta-ads-otel-exporter.<ext>
- Fixtures: capture real (sanitized) Meta API JSON responses as `testdata/*.json` fixtures for parsing tests, including nested `actions`/`purchase_roas` arrays
- Integration QA: `docker compose up -d`, wait for exporter to complete one poll cycle, query Mimir via its Prometheus-compatible query API for `meta_ads_spend` etc.

## Execution strategy
### Parallel execution waves
> Target 5-8 todos per wave. Fewer than 3 (except the final) means you under-split.

**Wave 1 (foundation, parallel)**: Config system, Meta API client skeleton + auth, OTel SDK exporter wiring, Docker Compose Mimir setup
**Wave 2 (core logic, depends on Wave 1)**: Meta API insights fetch + pagination, field extraction/classification, metric conversion to OTel gauges
**Wave 3 (integration, depends on Wave 2)**: Rate-limit/backoff logic, daemon poll loop, health endpoints, graceful shutdown
**Wave 4 (polish, depends on Wave 3)**: Empty-result/dropout handling, OTLP export failure handling, token-expiry handling
**Final wave**: F1-F4 verification (after all todos)

### Dependency matrix
| Todo | Depends on | Blocks | Can parallelize with |
| --- | --- | --- | --- |
| 1. Config system | none | 5,6,7 | 2,3,4 |
| 2. Meta API client + auth | none | 5 | 1,3,4 |
| 3. OTel exporter wiring | none | 6 | 1,2,4 |
| 4. Docker Compose Mimir | none | 9 (integration QA) | 1,2,3 |
| 5. Insights fetch + pagination | 1,2 | 6 | - |
| 6. Field extraction/classification | 3,5 | 7 | - |
| 7. Rate-limit/backoff logic | 1,5 | 8 | - |
| 8. Daemon poll loop + health endpoints | 6,7 | 9 | - |
| 9. Empty/dropout/export-failure/token-expiry handling | 8 | F1-F4 | - |

## Todos
> Implementation + Test = ONE todo. Never separate.
<!-- APPEND TASK BATCHES BELOW THIS LINE WITH edit/apply_patch - never rewrite the headers above. -->

- [ ] 1. Configuration system (YAML + env vars)
  What to do: Create `internal/config/config.go` defining a `Config` struct (AccountID, PollInterval, OTLPEndpoint, HealthPort, LogLevel). Load from `./config.yaml` (override via `--config` flag) using `gopkg.in/yaml.v3`. Load `META_ACCESS_TOKEN` and optional `OTEL_AUTH_HEADER` from env vars via `os.Getenv`. Validate on load: AccountID non-empty and matches `^act_\d+$`, PollInterval > 0, OTLPEndpoint parses as valid URL via `url.Parse`, META_ACCESS_TOKEN non-empty. Return descriptive error on any validation failure (e.g. "config: META_ACCESS_TOKEN env var is required").
  Must NOT do: Do not validate endpoint reachability (no network calls at config load time). Do not add config fields beyond what's listed.
  Parallelization: Wave 1 | Blocked by: none | Blocks: 5,6,7
  References: go.mod (module meta-marketing-exporter, go 1.25.0); cmd/main.go (empty, entrypoint will call config.Load())
  Acceptance criteria (agent-executable): `go build ./...` exits 0. `go test ./internal/config/...` exits 0 with a test asserting: (a) valid YAML+env loads without error, (b) missing META_ACCESS_TOKEN returns error containing "META_ACCESS_TOKEN", (c) malformed AccountID (e.g. "123") returns error containing "account".
  QA scenarios: Happy - `go test -run TestConfig_ValidLoad ./internal/config/...` passes. Failure - `go test -run TestConfig_MissingToken ./internal/config/...` passes, asserting error is returned (not panic). Evidence: .omo/evidence/task-1-meta-ads-otel-exporter.txt (paste `go test -v` output)
  Commit: Y | feat(config): add YAML+env config loader with validation

- [ ] 2. Meta API client skeleton with authentication
  What to do: Create `internal/meta/client.go` with a `Client` struct wrapping `net/http.Client`, base URL `https://graph.facebook.com/v21.0`, and the access token. Implement `NewClient(token string) *Client`. Implement a generic `doRequest(ctx, method, path string, params url.Values) ([]byte, error)` that appends `access_token` param, executes the request, and on non-2xx or a JSON body containing `{"error": {...}}`, parses the error object into a typed `MetaAPIError{Code int, Message string, Type string}` and returns it as the error (do not swallow the code/message).
  Must NOT do: Do not implement insights or campaign-listing logic here (that's todo 5). Do not hardcode the account ID.
  Parallelization: Wave 1 | Blocked by: none | Blocks: 5
  References: https://developers.facebook.com/docs/marketing-api/reference/ad-campaign-group (campaign node); https://developers.facebook.com/docs/marketing-api/error-reference (error codes 4, 17, 80000-80004, 613, 190, 102)
  Acceptance criteria (agent-executable): `go test ./internal/meta/...` exits 0 with tests using `httptest.NewServer` asserting: (a) a 200 response with valid JSON is parsed and returned, (b) a response body `{"error":{"code":190,"message":"token expired"}}` (regardless of HTTP status) is surfaced as `MetaAPIError{Code: 190}`.
  QA scenarios: Happy - `go test -run TestClient_SuccessfulRequest ./internal/meta/...` passes against a mock httptest server returning valid JSON. Failure - `go test -run TestClient_ErrorBodyParsing ./internal/meta/...` passes, asserting `errors.As(err, &MetaAPIError{})` succeeds and `Code == 190`. Evidence: .omo/evidence/task-2-meta-ads-otel-exporter.txt
  Commit: Y | feat(meta): add authenticated API client with typed error parsing

- [ ] 3. OpenTelemetry OTLP/HTTP exporter wiring
  What to do: Add dependency `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp` to go.mod (`go get go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp`). Create `internal/otel/exporter.go` with `NewMeterProvider(ctx context.Context, endpoint string, authHeader string) (*sdkmetric.MeterProvider, error)` that configures `otlpmetrichttp.New` with the given endpoint (parse host+path, use `WithEndpoint`/`WithURLPath` as needed) and optional auth header via `WithHeaders`, wraps it in a `sdkmetric.NewPeriodicReader`, and returns a `sdkmetric.NewMeterProvider` with `service.name=meta-ads-exporter` resource attribute. Register 12 gauge instruments (9 simple + 3 derived, per Decision 4) via `meter.Float64ObservableGauge` with names `meta_ads.spend`, `meta_ads.impressions`, `meta_ads.clicks`, `meta_ads.ctr`, `meta_ads.cpm`, `meta_ads.cpc`, `meta_ads.reach`, `meta_ads.frequency`, `meta_ads.roas`, `meta_ads.conversions`, `meta_ads.conversion_rate`, `meta_ads.cost_per_conversion`.
  Must NOT do: Do not implement the callback/observation logic here (that's todo 6/8's job to register callbacks). Do not use gRPC exporter.
  Parallelization: Wave 1 | Blocked by: none | Blocks: 6
  References: go.mod:10-13 (existing otel/otel-metric/otel-sdk v1.44.0); OTel Go metric SDK docs (go.opentelemetry.io/otel/sdk/metric)
  Acceptance criteria (agent-executable): `go build ./...` exits 0 after `go get` adds the exporter dependency (verify via `grep otlpmetrichttp go.mod` returns a match). `go test ./internal/otel/...` exits 0 with a test asserting `NewMeterProvider` returns a non-nil provider and no error for a valid endpoint URL, and returns an error for a malformed endpoint (e.g. empty string).
  QA scenarios: Happy - `go test -run TestNewMeterProvider_Valid ./internal/otel/...` passes. Failure - `go test -run TestNewMeterProvider_InvalidEndpoint ./internal/otel/...` passes. Evidence: .omo/evidence/task-3-meta-ads-otel-exporter.txt
  Commit: Y | feat(otel): wire OTLP/HTTP exporter and gauge instruments

- [ ] 4. Docker Compose: Grafana Mimir + exporter service
  What to do: Populate `docker-compose.yaml` with two services: `mimir` (image `grafana/mimir:latest`, single-process mode via `-target=all`, mounted local config enabling OTLP ingestion on `/otlp/v1/metrics`, exposed port 9009 for query API and 4318 for OTLP HTTP receiver, local filesystem storage volume, no auth), and `exporter` (build from local `Dockerfile`, depends_on mimir, env `OTEL_ENDPOINT=http://mimir:4318/otlp/v1/metrics`, mounts `./config.yaml`). Create a minimal `mimir-config.yaml` alongside docker-compose.yaml with `-target=all`, filesystem blocks storage, and `distributor.otlp` ingestion enabled.
  Must NOT do: Do not configure Mimir HA, multi-tenancy, or auth (dev-only). Do not add Grafana/Prometheus/Loki services (out of scope).
  Parallelization: Wave 1 | Blocked by: none | Blocks: 9 (integration QA)
  References: docker-compose.yaml (currently empty, 0 lines); Dockerfile (existing, check base image for exporter build stage compatibility); Grafana Mimir OTLP ingestion docs (distributor.otlp config, single-process `-target=all` mode)
  Acceptance criteria (agent-executable): `docker compose config` exits 0 (valid compose syntax). `docker compose up -d mimir && sleep 15 && curl -sf http://localhost:9009/ready` returns HTTP 200. `docker compose down` cleans up.
  QA scenarios: Happy - `docker compose up -d mimir && sleep 15 && curl -s http://localhost:9009/ready` returns "ready". Failure - stop mimir mid-run (`docker compose stop mimir`), confirm `docker compose logs exporter` shows retry/backoff log lines rather than a crash. Evidence: .omo/evidence/task-4-meta-ads-otel-exporter.txt (curl output + compose logs)
  Commit: Y | feat(compose): add Grafana Mimir dev stack with OTLP ingestion

- [ ] 5. Campaign list + insights fetch with pagination
  What to do: In `internal/meta/insights.go`, implement `ListCampaigns(ctx, accountID string) ([]Campaign, error)` calling `GET /act_{id}/campaigns?fields=id,name` and `FetchTodayInsights(ctx, accountID string) ([]InsightRow, error)` calling `GET /act_{id}/insights?level=campaign&date_preset=today&fields=campaign_id,campaign_name,spend,account_currency,date_start,date_stop,impressions,clicks,ctr,cpm,cpc,reach,frequency,actions,action_values,purchase_roas`. Both methods must follow `paging.next` cursors in the JSON response, accumulating all pages into the returned slice, until `paging.next` is absent.
  Must NOT do: Do not request ad-set or ad-level data (`level` param must always be `campaign`). Do not request fields outside the list above.
  Parallelization: Wave 2 | Blocked by: 1,2 | Blocks: 6
  References: internal/config/config.go (todo 1, provides AccountID); internal/meta/client.go (todo 2, provides doRequest); Meta Marketing API campaign insights reference (level=campaign, date_preset=today, action_type breakdown for actions/action_values/purchase_roas)
  Acceptance criteria (agent-executable): `go test ./internal/meta/...` exits 0 with a test using a fixture file `testdata/insights_paginated_page1.json` and `testdata/insights_paginated_page2.json` (both with realistic `actions`/`purchase_roas` nested arrays), asserting `FetchTodayInsights` returns the combined row count from both pages and correctly parses at least one row with a non-empty `actions` array.
  QA scenarios: Happy - `go test -run TestFetchTodayInsights_Pagination ./internal/meta/...` passes, asserting total rows = sum across both fixture pages. Failure - `go test -run TestFetchTodayInsights_EmptyResult ./internal/meta/...` passes using a fixture with empty `data: []`, asserting an empty (not nil-panic) slice is returned. Evidence: .omo/evidence/task-5-meta-ads-otel-exporter.txt
  Commit: Y | feat(meta): fetch campaign list and today's insights with pagination

- [ ] 6. Field extraction and metric conversion (nested action arrays)
  What to do: In `internal/meta/extract.go`, implement `ExtractMetrics(row InsightRow) CampaignMetrics` where `CampaignMetrics` has fields for all 12 gauges (per Decision 4) plus labels (campaign_id, campaign_name, account_currency, date_start, date_stop). Simple fields (spend, impressions, clicks, ctr, cpm, cpc, reach, frequency) parse directly from string-to-float64 (Meta returns these as JSON strings). For `actions` array: sum `value` fields where `action_type` is in `["purchase", "offsite_conversion.fb_pixel_purchase"]` to produce `Conversions`. For `purchase_roas` array: sum `value` fields similarly to produce `Roas` (0 if array empty/absent). Derive `ConversionRate = Conversions / Clicks` (0 if Clicks is 0, avoid divide-by-zero panic). Derive `CostPerConversion = Spend / Conversions` (0 if Conversions is 0). In `internal/otel/observe.go`, implement `RecordMetrics(metrics []CampaignMetrics)` that sets each of the 12 gauge instruments (from todo 3) via their observable callback, attaching attributes `campaign_id`, `campaign_name`, `account_id`, `currency` to each observation. If `metrics` is empty for a campaign that previously reported, do NOT emit a callback for it (Decision 9: dropout = stale, not zeroed); if a campaign has all-zero delivery today, emit 0 for numeric gauges (Decision 9).
  Must NOT do: Do not panic on missing/malformed numeric strings (default to 0 and log a warning). Do not emit `date_start`/`date_stop` as gauges (they are labels/metadata per Decision 4, not numeric metrics — attach as attributes or drop, do not create a `meta_ads.date_start` gauge).
  Parallelization: Wave 2 | Blocked by: 3,5 | Blocks: 7
  References: internal/meta/insights.go (todo 5, provides InsightRow); internal/otel/exporter.go (todo 3, provides gauge instruments); Decision 4 and Decision 8 in .omo/drafts/meta-ads-otel-exporter.md (field classification and extraction logic)
  Acceptance criteria (agent-executable): `go test ./internal/meta/... ./internal/otel/...` exits 0 with tests asserting: (a) a fixture row with `actions: [{action_type: "purchase", value: "5"}, {action_type: "link_click", value: "100"}]` yields `Conversions == 5` (link_click excluded), (b) `Clicks == 0` yields `ConversionRate == 0` without panic, (c) a row with empty `data` yields all-zero numeric fields, not an error.
  QA scenarios: Happy - `go test -run TestExtractMetrics_ActionSumming ./internal/meta/...` passes. Failure - `go test -run TestExtractMetrics_DivideByZero ./internal/meta/...` passes without panic (assert via `assert.NotPanics` or explicit recover check). Evidence: .omo/evidence/task-6-meta-ads-otel-exporter.txt
  Commit: Y | feat(meta): extract nested action metrics and derive computed fields

- [ ] 7. Rate-limit detection and exponential backoff
  What to do: In `internal/meta/backoff.go`, implement `IsThrottled(err error) (bool, time.Duration)` that checks if `err` is a `MetaAPIError` (from todo 2) with `Code` in `[4, 17, 80000, 80001, 80002, 80003, 80004, 613]`, returning `true` and a suggested delay. Implement `ParseRetryHeader(headers http.Header) (time.Duration, bool)` that reads the `X-Business-Use-Case-Usage` header, parses its JSON value, and extracts `estimated_time_to_regain_access` (in minutes) if present, converting to `time.Duration`. Implement `WithBackoff(ctx context.Context, fn func() error) error` wrapping any call with: on throttle error, sleep for the header-derived duration (or exponential backoff starting at 1s, doubling, capped at 60s, with jitter, if header absent), retry up to 5 times; on non-throttle transient error (network timeout, 5xx), retry up to 3 times with 1s/2s/4s backoff; on auth error (code 190 or 102), return immediately without retry.
  Must NOT do: Do not trigger backoff based on HTTP status 429 alone (Decision 7 — Meta rarely uses 429; must check error codes in body). Do not retry auth errors.
  Parallelization: Wave 2 | Blocked by: 1,5 | Blocks: 8
  References: internal/meta/client.go (todo 2, provides MetaAPIError); Decision 7 in .omo/drafts/meta-ads-otel-exporter.md (error codes 4,17,80000-80004,613; header semantics); Meta rate limiting docs (X-Business-Use-Case-Usage header format)
  Acceptance criteria (agent-executable): `go test ./internal/meta/...` exits 0 with tests: (a) `IsThrottled` returns true for `MetaAPIError{Code: 17}`, false for `MetaAPIError{Code: 190}`, (b) `ParseRetryHeader` correctly extracts a duration from a sample header JSON fixture, (c) `WithBackoff` retries a function that fails twice with a throttle error then succeeds, returning nil after exactly 2 retries (assert via call counter).
  QA scenarios: Happy - `go test -run TestWithBackoff_RetriesOnThrottle ./internal/meta/...` passes. Failure - `go test -run TestWithBackoff_NoRetryOnAuthError ./internal/meta/...` passes, asserting the function is called exactly once (no retry) for code 190. Evidence: .omo/evidence/task-7-meta-ads-otel-exporter.txt
  Commit: Y | feat(meta): add error-code-based throttle detection with backoff

- [ ] 8. Daemon poll loop, health endpoints, graceful shutdown
  What to do: In `cmd/main.go`, wire everything: load config (todo 1), create Meta client (todo 2), create OTel meter provider (todo 3), start an HTTP server on `config.HealthPort` with `/healthz` (always returns 200 if process alive) and `/readyz` (returns 200 only after the first successful poll cycle completes; 503 before that or if the last 3 consecutive polls failed). Implement the poll loop: `ticker := time.NewTicker(config.PollInterval)`, on each tick call `ListCampaigns` + `FetchTodayInsights` (wrapped in `WithBackoff` from todo 7), then `ExtractMetrics` (todo 6) + `RecordMetrics` (todo 6). On `SIGINT`/`SIGTERM`, stop the ticker, flush the OTel meter provider (`provider.Shutdown(ctx)` with 30s timeout context), close the HTTP server gracefully, then exit 0.
  Must NOT do: Do not let a single failed poll cycle crash the process (log and continue to next tick) — except for auth errors (code 190/102) which per Decision 11 should log fatal and exit non-zero. Do not skip the graceful shutdown timeout.
  Parallelization: Wave 3 | Blocked by: 6,7 | Blocks: 9
  References: internal/config, internal/meta, internal/otel (todos 1,2,3,5,6,7); Decision 11 in .omo/drafts/meta-ads-otel-exporter.md (token expiry = fatal exit)
  Acceptance criteria (agent-executable): `go build -o bin/exporter ./cmd/main.go` exits 0. Running the binary with a valid config and `curl -s -o /dev/null -w "%{http_code}" localhost:8080/healthz` returns `200` within 2 seconds of start. Sending `SIGTERM` to the process causes it to exit with code 0 within 30 seconds (assert via `timeout 35s kill -TERM $PID && wait $PID; echo $?` returns 0).
  QA scenarios: Happy - start binary, `curl localhost:8080/healthz` returns 200, `curl localhost:8080/readyz` returns 503 before first poll and 200 after (poll interval temporarily set to 1s via test config for fast QA). Failure - inject an auth error (mock token), confirm process logs "fatal" and exits non-zero (`echo $?` != 0), and confirm SIGTERM during an in-flight poll waits for it to finish (or times out at 30s) before exiting. Evidence: .omo/evidence/task-8-meta-ads-otel-exporter.txt
  Commit: Y | feat(cmd): wire daemon poll loop with health checks and graceful shutdown

- [ ] 9. Operational edge cases: empty results, export failures, integration QA
  What to do: Verify and harden: (a) empty-delivery campaigns emit 0-value gauges (per Decision 9, already implemented in todo 6 — add explicit test coverage here), (b) OTLP export failure (Mimir unreachable) triggers retry with backoff (max 3 attempts) then logs error and drops the batch without crashing the daemon (implement in `internal/otel/exporter.go`, wrapping the `PeriodicReader`'s export call or using the exporter's built-in retry config `otlpmetrichttp.WithRetry`), (c) run full end-to-end integration QA via docker-compose: start the full stack, wait for 2 poll cycles (~20s with a fast test interval), query Mimir's Prometheus-compatible API for `meta_ads_spend` and confirm at least one result with a `campaign_id` label.
  Must NOT do: Do not add retry logic beyond max 3 attempts for OTLP export (avoid unbounded retry loops). Do not block the next Meta poll cycle while retrying a stuck OTLP export (run export retry with its own timeout separate from the poll ticker).
  Parallelization: Wave 4 | Blocked by: 8 | Blocks: F1-F4
  References: internal/otel/exporter.go (todo 3); docker-compose.yaml (todo 4); cmd/main.go (todo 8); otlpmetrichttp.WithRetry config docs
  Acceptance criteria (agent-executable): `go test ./internal/otel/...` exits 0 with a test simulating an unreachable OTLP endpoint (e.g. point to an unused port) and asserting the exporter logs an error and returns without panicking within the retry+timeout budget (assert test completes in <10s, not hanging). Integration: `docker compose up -d && sleep 30 && curl -s 'http://localhost:9009/prometheus/api/v1/query?query=meta_ads_spend' | jq '.data.result | length'` outputs a number ≥ 0 (≥1 if test account has campaigns).
  QA scenarios: Happy - full docker-compose stack up, query returns `meta_ads_spend` series with `campaign_id` label populated. Failure - stop the `mimir` container mid-run (`docker compose stop mimir`), confirm exporter logs show export retry/failure messages (not a crash) via `docker compose logs exporter | grep -i "export"`, then `docker compose start mimir` and confirm exporter resumes successful export on next cycle. Evidence: .omo/evidence/task-9-meta-ads-otel-exporter.txt (full docker compose logs + jq query output)
  Commit: Y | feat(otel): harden export failure handling and add integration QA

## Final verification wave
> Runs in parallel after ALL todos. ALL must APPROVE. Surface results and wait for the user's explicit okay before declaring complete.
- [ ] F1. Plan compliance audit: Verify all Must Have items are implemented (12 gauge metrics, rate-limit error codes, pagination, backoff, graceful shutdown) and all Must NOT Have items are absent (no ad-set level, no 429-only detection, no flat-scalar conversions assumption). Evidence: code review checklist in .omo/evidence/F1-compliance-audit.md
- [ ] F2. Code quality review: Go vet, golint, no TODO/FIXME comments, consistent error handling, proper context usage, no data races under go test -race. Evidence: static analysis output in .omo/evidence/F2-code-quality.txt
- [ ] F3. Real manual QA: Full stack test with real Meta API token + real ad account (sanitized logs). Verify: metrics appear in Mimir, campaign_id labels are correct, rate-limit backoff triggers on actual throttle (if testable), graceful shutdown works under load. Evidence: QA session transcript in .omo/evidence/F3-manual-qa.md
- [ ] F4. Scope fidelity: Confirm deliverable matches original user requirements (17 fields to OTLP gauges, campaign level, 5-15min poll, JSON logs, health endpoint, docker-compose Mimir). Evidence: requirements traceability matrix in .omo/evidence/F4-scope-fidelity.md

## Commit strategy
- Wave 1 (todos 1-4): Individual commits per todo, merge to main via fast-forward after each passes tests
- Wave 2 (todos 5-6): Individual commits, can be squash-merged as a "feat: Meta API integration" PR if preferred
- Wave 3 (todos 7-8): Individual commits, 8 is the "working daemon" milestone - tag as v0.1.0-alpha
- Wave 4 (todo 9): Single commit for operational hardening
- Final verification: Only documentation commits (evidence files), no code changes

## Success criteria
1. **Functional**: `docker compose up -d && sleep 60 && curl 'http://localhost:9009/prometheus/api/v1/query?query=meta_ads_spend'` returns metrics with campaign_id labels from real Meta ad account
2. **Operational**: Process survives Meta API throttling (logs retry, continues polling), survives Mimir downtime (logs export failure, resumes on recovery), responds to SIGTERM within 30s
3. **Maintainable**: All components have >80% test coverage (`go test -cover ./...`), zero go vet/golint issues, README with quickstart instructions
4. **Scoped**: Exactly 12 gauge metrics (9 simple + 3 derived), exactly campaign level, no scope creep beyond approved Must Have list
