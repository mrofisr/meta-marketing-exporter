# F3 - Real Manual QA

## What was verified end-to-end (this session)
- `docker compose config` — valid YAML, EXIT 0
- Mimir container starts, `/ready` returns HTTP 200 "ready"
- Mimir OTLP endpoint `/otlp/v1/metrics` reachable (405 on GET, expects POST — correct)
- Mimir Prometheus-compatible query API responds with valid JSON for `meta_ads_spend` query
- Exporter binary builds (`go build -o bin/exporter ./cmd/main.go`, EXIT 0)
- Exporter starts, loads config, starts health server, begins polling
- Exporter correctly detects Meta auth error (code 190 with placeholder token) and fatal-exits per Decision 11 — this IS the expected/designed behavior, not a bug
- `docker compose down` cleans up without error
- OTLP export against an unreachable endpoint: bounded retry (500ms/2s/6s budget), completes in <1s, no panic, no hang (unit test `TestNewMeterProvider_UnreachableEndpoint`)

## What requires the user's real credentials (cannot be verified in this session)
- End-to-end metric flow with a REAL Meta ad account: campaign data → Mimir → queryable `meta_ads_spend` with populated `campaign_id` label
- Real rate-limit throttle triggering actual backoff (requires sustained real traffic against Meta's API)
- Full Mimir-down/Mimir-up recovery cycle while the exporter is actively polling real data (blocked because the placeholder token causes fatal-exit before any OTLP export is attempted)
- Graceful SIGTERM shutdown timing under real in-flight Meta API calls

## Recommendation for user
Before production use:
1. Set a real `META_ACCESS_TOKEN` in `.env`
2. Set a real `account_id` in `config.docker.yaml` (or `config.yaml`)
3. Run `docker compose up -d` and confirm `curl http://localhost:9009/prometheus/api/v1/query?query=meta_ads_spend` returns actual campaign data within ~2 poll cycles (20 min at default 10-min interval)
4. Optionally test recovery: `docker compose stop mimir`, observe exporter logs show retry/drop (not crash), `docker compose start mimir`, confirm resumption

## Verdict: CONDITIONAL PASS
All testable-without-credentials paths verified. End-to-end real-data flow requires user's own Meta token/account per the plan's scope (this was never something the agent could obtain — it's the user's private credential).
