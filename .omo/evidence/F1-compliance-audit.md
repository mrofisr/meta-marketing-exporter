# F1 - Plan Compliance Audit

## Must Have Items (from plan scope) - Verification

### Core Functionality
- [x] Meta Marketing API client for one ad account - `internal/meta/client.go`
- [x] Campaign-level insights fetch with `date_preset=today`, 10-minute poll - `internal/meta/insights.go`, `cmd/main.go`
- [x] Field extraction with correct classification:
  - [x] 9 simple gauges: spend, impressions, clicks, ctr, cpm, cpc, reach, frequency, roas - `internal/meta/extract.go`
  - [x] 3 derived gauges: conversions, conversion_rate, cost_per_conversion - `internal/meta/extract.go`
  - [x] Attributes: campaign_id, campaign_name, account_currency, date_start, date_stop - `internal/otel/observe.go`
- [x] Nested action array extraction (purchase/offsite_conversion.fb_pixel_purchase) - `internal/meta/extract.go:58-70`
- [x] OpenTelemetry gauge metrics with `meta_ads_*` naming - `internal/otel/instruments.go`
- [x] OTLP/HTTP export to Mimir - `internal/otel/exporter.go`
- [x] Rate-limit detection via error codes (4,17,80000-80004,613) NOT HTTP 429 - `internal/meta/backoff.go:27-28`
- [x] Exponential backoff with X-Business-Use-Case-Usage header parsing - `internal/meta/backoff.go:48-77`
- [x] Pagination for campaigns list and insights - `internal/meta/insights.go:33-39,66-72`
- [x] Empty result = emit 0 gauges - `internal/meta/extract_test.go:TestExtractMetrics_EmptyDeliveryEmitsZeros`
- [x] Campaign dropout = leave stale - `internal/otel/observe.go:47-49`
- [x] OTLP export failure: retry max 3, drop batch, log - `internal/otel/exporter.go:15-18,31-32`
- [x] Token expiry (190,102): fatal exit - `cmd/main.go:147-151,171-177`
- [x] YAML config + env var secrets - `internal/config/config.go`
- [x] JSON structured logging (slog) - `cmd/main.go:26-39`
- [x] /healthz and /readyz on port 8080 - `cmd/main.go:78-92`
- [x] Graceful shutdown (SIGTERM, 30s timeout) - `cmd/main.go:193-209`
- [x] Docker Compose: Mimir + exporter - `docker-compose.yaml`
- [x] Unit tests for Meta API parsing, metric conversion, backoff - all `*_test.go` files

## Must NOT Have Items - Verification

- [x] No ad set/ad level metrics (campaign only) - verified: `insights.go:60` hardcoded `level=campaign`
- [x] No multiple accounts - verified: single `cfg.AccountID` in `main.go`
- [x] No historical backfill beyond "today" - verified: `insights.go:60` hardcoded `date_preset=today`
- [x] No token refresh/rotation - verified: no refresh logic in codebase
- [x] No production Mimir HA/auth - verified: `mimir-config.yaml` single-process, `docker-compose.yaml:12` auth disabled
- [x] No sub-minute polling - verified: default 10m in `config.yaml`
- [x] No Counter metrics - verified: all `Float64ObservableGauge` in `instruments.go`
- [x] No gRPC OTLP - verified: `exporter.go` uses `otlpmetrichttp` only
- [x] No Kubernetes manifests - verified: no k8s files in repo
- [x] No HTTP 429-only detection - verified: `backoff.go:27` checks error codes, not status
- [x] No flat-scalar assumption for conversions/roas - verified: `extract.go:58-87` iterates action arrays

## Verdict: PASS
All Must Have items implemented. All Must NOT Have guardrails respected.
