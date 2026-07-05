# F4 - Scope Fidelity (Requirements Traceability)

## Original user requirements (from grilling session) -> Implementation

| Requirement | Delivered | Location |
|---|---|---|
| Single binary, Go | Yes | `go build -o bin/exporter ./cmd/main.go` produces one static binary |
| Single ad account | Yes | `Config.AccountID` single field |
| 17 fields (later corrected to 12 gauges + 5 labels by Metis review, approved by user via plan approval) | Yes | 12 gauges in `instruments.go`, 5 labels in `observe.go` |
| Campaign level only | Yes | `level=campaign` hardcoded in `insights.go` |
| Poll every 5-15min (10min adopted default) for "today" | Yes | `date_preset=today`, `poll_interval: 10m` default |
| OTel Metrics as Gauges (not Counters) | Yes | All 12 `Float64ObservableGauge` |
| OTLP to Grafana Mimir | Yes | `otlpmetrichttp` exporter, docker-compose Mimir stack |
| Config: env vars for secrets + YAML for rest | Yes | `META_ACCESS_TOKEN`/`OTEL_AUTH_HEADER` env, `config.yaml` for the rest |
| Handle rate limit/error conditions | Yes | Error-code-based (not 429) throttle detection + backoff |
| JSON structured logging | Yes | `log/slog` JSON handler |
| Health check endpoint | Yes | `/healthz`, `/readyz` on port 8080 |
| Docker Compose with Mimir | Yes | `docker-compose.yaml`, `mimir-config.yaml` |
| Test strategy: tests-after | Yes | All packages implemented first, then table-driven tests |

## Deviations from original ask (all user-approved during planning)
- 17 "fields" -> 12 actual gauges + 5 labels/attributes (date_start/date_stop are not gauges; campaign_id/campaign_name/currency are attributes, not metrics) — corrected by Metis gap analysis, reflected in the approved plan
- Rate-limit detection corrected from HTTP-429-based (user's original mental model) to error-code-based (Meta's actual behavior) — corrected by Metis, necessary for correctness

## Scope creep check
- No ad-set/ad-level metrics added
- No multi-account support added
- No alerting/dashboards added
- No Kubernetes manifests added
- No gRPC OTLP added

## Verdict: PASS
Delivered scope matches the approved plan exactly. The two corrections (field classification, rate-limit semantics) were identified by Metis before implementation and are necessary fixes to Meta's actual API behavior, not scope creep — without them the exporter would have been built against a fictional API contract.
