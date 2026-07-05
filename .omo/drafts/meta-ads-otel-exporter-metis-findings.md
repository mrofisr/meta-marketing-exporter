# Metis Gap Analysis - meta-ads-otel-exporter

## Critical Gaps to Address

### 1. Meta API Field Structure (HIGH PRIORITY)
**Issue**: Plan treats `conversions`, `roas`, `cost_per_conversion` as flat scalars. They are actually nested action arrays.

**Reality**:
- `actions` returns array: `[{action_type: "purchase", value: "5"}, {action_type: "link_click", value: "123"}]`
- `purchase_roas` (not `roas`) returns array by action_type
- `conversion_rate` is NOT a standard Meta field - must be derived
- `cost_per_conversion` may be `cost_per_action_type` array

**Resolution**:
- Specify which `action_type` counts as "conversion" (recommend: `purchase`, `offsite_conversion.fb_pixel_purchase`)
- Define extraction: sum values across selected action_types, or pick specific one
- Derive `conversion_rate` as `conversions / clicks`
- Verify field names at campaign level endpoint

### 2. Rate Limit Detection (HIGH PRIORITY)
**Issue**: Plan assumes HTTP 429. Meta returns throttle as HTTP 200/400 with error codes.

**Reality**:
- Throttle error codes: 4, 17, 80000-80004, 613
- Returned in JSON body: `{"error": {"code": 17, "message": "..."}}`
- `X-Business-Use-Case-Usage` header has `estimated_time_to_regain_access` (minutes) when throttled

**Resolution**:
- Parse response body for error codes, not just HTTP status
- Extract retry delay from header when present
- Default exponential backoff if header absent

### 3. Field Classification (MEDIUM PRIORITY)
**Issue**: 17 fields mix gauges, labels, and non-numeric data.

**Correct classification**:
- **Gauges (9)**: spend, impressions, clicks, ctr, cpm, cpc, reach, frequency, (derived: conversions, conversion_rate, cost_per_conversion)
- **Attributes/labels**: campaign_id, campaign_name, account_currency
- **Metadata (not gauges)**: date_start, date_stop (use as labels or drop)
- **Complex**: roas (nested, needs extraction logic)

### 4. Missing Dependencies
- Need `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp`
- Current go.mod only has SDK, not exporter

### 5. Operational Gaps
- Empty result behavior (campaign with no delivery today): emit 0, skip, or leave stale?
- Campaign dropout (was active, now inactive): zero out or go stale?
- OTLP export failure: retry with backoff or drop?
- Token expiry: crash or log-and-continue?
- Pagination: both campaigns list and insights paginate (cursor-based)
- Timezone: `date_preset=today` uses account timezone, affects midnight rollover

### 6. Acceptance Criteria Missing
No component has agent-executable verification defined. Need concrete commands with expected outputs.

## Directives for Plan Writing
- Add field extraction logic for nested action arrays
- Specify error-code-based throttle detection
- Add OTLP exporter dependency as explicit step
- Define empty/dropout/failure behaviors
- Add pagination handling
- Write concrete acceptance criteria per component (exact curl/go test commands)
