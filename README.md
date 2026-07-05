# meta-marketing-exporter

A single Go binary that polls the Meta Marketing API for one ad account's
campaign-level insights and exports them as OpenTelemetry gauge metrics via
OTLP/HTTP to Grafana Mimir.

Built for cost monitoring, performance analysis, and campaign optimization
dashboards/alerting on top of Mimir/Grafana.

## What it does

- Polls Meta's `/insights` endpoint at `level=campaign`, `date_preset=today`,
  every 10 minutes (configurable).
- Extracts 12 metrics per campaign as OpenTelemetry gauges:
  - **Simple**: `spend`, `impressions`, `clicks`, `ctr`, `cpm`, `cpc`,
    `reach`, `frequency`, `roas` (from Meta's `purchase_roas` action array)
  - **Derived**: `conversions` (summed from the `actions` array for
    `purchase` / `offsite_conversion.fb_pixel_purchase`), `conversion_rate`
    (`conversions / clicks`), `cost_per_conversion` (`spend / conversions`)
- Attaches `campaign_id`, `campaign_name`, `account_id`, `currency` as metric
  attributes.
- Exports via OTLP/HTTP to a Mimir (or any OTLP-compatible) endpoint.
- Handles Meta's actual rate-limiting behavior: throttling shows up as
  error codes (`4`, `17`, `80000`-`80004`, `613`) in the JSON body, not
  HTTP 429. Backoff honors the `X-Business-Use-Case-Usage` header when
  present, otherwise falls back to exponential backoff with jitter.
- Exposes `/healthz` (process alive) and `/readyz` (first poll cycle
  completed) on port 8080.
- Graceful shutdown on `SIGINT`/`SIGTERM` (30s timeout).

## What it does NOT do

- No ad-set or ad-level metrics (campaign level only).
- No multi-account support (single ad account).
- No historical backfill (only "today", in the ad account's timezone).
- No token refresh/rotation (bring your own long-lived token; the daemon
  exits non-zero on auth failure and expects manual token rotation).
- The bundled `docker-compose.yaml` Mimir stack is for local development
  only — no HA, no auth, no multi-tenancy.

## Quick start (local dev with Docker Compose)

```bash
cp .env.example .env
# edit .env and set META_ACCESS_TOKEN

cp config.yaml config.local.yaml
# edit config.local.yaml and set your account_id (must match ^act_\d+$)

docker compose up -d
```

This starts:
- `mimir` — single-process Grafana Mimir, filesystem storage, no auth.
  Query API + `/ready` + OTLP ingestion all served on port `9009`
  (Mimir does not run a separate OTLP port; OTLP is served at
  `/otlp/v1/metrics` on the main HTTP port).
- `exporter` — this binary, built from the local `Dockerfile`, using
  `config.docker.yaml` (points at `http://mimir:9009/otlp/v1/metrics`
  inside the compose network).

Verify Mimir is up:

```bash
curl -sf http://localhost:9009/ready
```

Query exported metrics once the exporter has completed a poll cycle:

```bash
curl -s 'http://localhost:9009/prometheus/api/v1/query?query=meta_ads_spend' | jq
```

(Note: OpenTelemetry dotted names like `meta_ads.spend` are transformed to
`meta_ads_spend` on Prometheus/Mimir ingestion.)

## Running the binary directly (no Docker)

```bash
export META_ACCESS_TOKEN=your_token_here
go build -o bin/exporter ./cmd
./bin/exporter --config config.yaml
```

`config.yaml` (non-secret settings):

```yaml
account_id: "act_XXXXXXXXX"
poll_interval: "10m"       # 5m-15m recommended; <5m risks rate limiting
otlp_endpoint: "http://localhost:4318/otlp/v1/metrics"
health_port: 8080
log_level: "info"          # debug | info | warn | error
```

Secrets are environment variables only, never in `config.yaml`:
- `META_ACCESS_TOKEN` (required) — long-lived Meta access token
- `OTEL_AUTH_HEADER` (optional) — e.g. `X-Scope-OrgID:tenant-1` for
  multi-tenant OTLP backends

## Project layout

```
cmd/                  entrypoint: wires config, Meta client, OTel exporter,
                       poll loop, health endpoints, graceful shutdown
internal/config/       YAML + env var config loading and validation
internal/meta/          Meta API client, typed error parsing, campaign +
                       insights fetch with pagination, rate-limit backoff,
                       nested action-array metric extraction
internal/otel/          OTLP/HTTP exporter wiring, gauge instrument registration
```

## Development

```bash
go build ./...
go test ./...
go vet ./...
```

Test strategy is tests-after: implementation first, then table-driven Go
tests per package. See `.omo/plans/meta-ads-otel-exporter.md` for the full
implementation plan, decisions, and acceptance criteria.

## Logging

Structured JSON logs via `log/slog`, `info` level by default. Log level is
configurable via `config.yaml`.
