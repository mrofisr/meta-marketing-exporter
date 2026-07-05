package meta

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"net/http"
	"time"
)

// sleepFunc is the function used to sleep between retries. It is a package-level
// variable so tests can override it to avoid real time.Sleep calls, keeping the
// test suite fast.
var sleepFunc = time.Sleep

// throttleCodes is the set of Meta API error codes that indicate the request
// was rate-limited / throttled. Per Decision 7, Meta reports throttling via
// these codes in the JSON response body, NOT via HTTP status 429.
//
//	4     - Application request limit reached
//	17    - User request limit reached
//	613   - Calls to this API have exceeded the rate limit
//	80000 - 80004 - Business-use-case rate limits (ads_insights, ads_management, etc.)
var throttleCodes = map[int]struct{}{
	4:     {},
	17:    {},
	613:   {},
	80000: {},
	80001: {},
	80002: {},
	80003: {},
	80004: {},
}

// authCodes is the set of Meta API error codes that indicate an
// authentication/authorization failure requiring human intervention (token
// rotation). These must NOT be retried (Decision 11).
//
//	190 - Access token expired / invalid
//	102 - Session key invalid or session expired
var authCodes = map[int]struct{}{
	190: {},
	102: {},
}

// defaultThrottleDelay is the suggested delay returned by IsThrottled when a
// throttle error is detected but no header-derived duration is available.
const defaultThrottleDelay = time.Minute

// backoff tuning constants.
const (
	throttleMaxRetries  = 5
	throttleBaseDelay   = 1 * time.Second
	throttleMaxDelay    = 60 * time.Second
	transientMaxRetries = 3
	transientBaseDelay  = 1 * time.Second
)

// IsThrottled reports whether err is (or wraps) a *MetaAPIError whose Code
// indicates rate-limiting/throttling. When throttled it also returns a default
// suggested delay (defaultThrottleDelay); callers with access to response
// headers should prefer ParseRetryHeader for a more accurate wait. When not
// throttled it returns (false, 0).
func IsThrottled(err error) (bool, time.Duration) {
	if err == nil {
		return false, 0
	}
	var apiErr *MetaAPIError
	if !errors.As(err, &apiErr) {
		return false, 0
	}
	if _, ok := throttleCodes[apiErr.Code]; ok {
		return true, defaultThrottleDelay
	}
	return false, 0
}

// IsAuthError reports whether err is (or wraps) a *MetaAPIError whose Code
// indicates an authentication failure that must not be retried.
func IsAuthError(err error) bool {
	var apiErr *MetaAPIError
	if !errors.As(err, &apiErr) {
		return false
	}
	_, ok := authCodes[apiErr.Code]
	return ok
}

// businessUseCaseEntry models a single entry in the X-Business-Use-Case-Usage
// header value, e.g. {"type": "ads_management", "estimated_time_to_regain_access": 15}.
// estimated_time_to_regain_access is expressed in minutes.
type businessUseCaseEntry struct {
	Type                        string `json:"type"`
	EstimatedTimeToRegainAccess int    `json:"estimated_time_to_regain_access"`
}

// ParseRetryHeader reads the X-Business-Use-Case-Usage header, which Meta
// returns as a JSON object keyed by ad-account id, each mapping to an array of
// usage entries, e.g.:
//
//	{"act_123": [{"type": "ads_management", "estimated_time_to_regain_access": 15}]}
//
// It extracts the largest estimated_time_to_regain_access (in minutes) across
// all entries and returns it as a time.Duration. It returns (0, false) if the
// header is absent, unparseable, or contains no positive wait time.
func ParseRetryHeader(headers http.Header) (time.Duration, bool) {
	if headers == nil {
		return 0, false
	}
	raw := headers.Get("X-Business-Use-Case-Usage")
	if raw == "" {
		return 0, false
	}

	var byAccount map[string][]businessUseCaseEntry
	if err := json.Unmarshal([]byte(raw), &byAccount); err != nil {
		return 0, false
	}

	maxMinutes := 0
	for _, entries := range byAccount {
		for _, e := range entries {
			if e.EstimatedTimeToRegainAccess > maxMinutes {
				maxMinutes = e.EstimatedTimeToRegainAccess
			}
		}
	}

	if maxMinutes <= 0 {
		return 0, false
	}
	return time.Duration(maxMinutes) * time.Minute, true
}

// WithBackoff wraps fn with retry/backoff logic tuned to Meta's error
// semantics:
//
//   - Throttle errors (IsThrottled): retried up to throttleMaxRetries times
//     using exponential backoff (1s, doubling, capped at 60s) with jitter.
//   - Auth errors (codes 190/102): returned immediately, never retried.
//   - Other (transient) errors: retried up to transientMaxRetries times with
//     1s/2s/4s backoff.
//
// The context is honored during every sleep: if ctx is cancelled while waiting,
// WithBackoff returns ctx.Err() immediately. WithBackoff returns nil as soon as
// fn succeeds, or the last error once retries are exhausted.
func WithBackoff(ctx context.Context, fn func() error) error {
	var lastErr error
	throttleAttempts := 0
	transientAttempts := 0

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		// Auth errors are terminal - do not retry.
		if IsAuthError(lastErr) {
			return lastErr
		}

		var delay time.Duration
		if throttled, suggested := IsThrottled(lastErr); throttled {
			if throttleAttempts >= throttleMaxRetries {
				return lastErr
			}
			delay = throttleDelay(throttleAttempts, suggested)
			throttleAttempts++
		} else {
			if transientAttempts >= transientMaxRetries {
				return lastErr
			}
			delay = transientBaseDelay * (1 << transientAttempts)
			transientAttempts++
		}

		if err := sleepWithContext(ctx, delay); err != nil {
			return err
		}
	}
}

// throttleDelay computes the wait before the next throttle retry. It uses
// exponential backoff (base 1s, doubling) capped at 60s, with up to 25% jitter
// added. The suggested delay from IsThrottled is used as a floor so we never
// wait less than Meta's default guidance.
func throttleDelay(attempt int, suggested time.Duration) time.Duration {
	backoff := throttleBaseDelay * (1 << attempt)
	if backoff > throttleMaxDelay {
		backoff = throttleMaxDelay
	}
	if suggested > 0 && suggested < backoff {
		// Prefer the exponential backoff when it already exceeds the default
		// suggestion; otherwise honor at least the suggested floor.
		suggested = backoff
	}
	base := backoff
	if suggested > base {
		base = suggested
	}
	// Add jitter: up to 25% of the base delay.
	jitter := time.Duration(rand.Int63n(int64(base)/4 + 1))
	return base + jitter
}

// sleepWithContext sleeps for d using the injectable sleepFunc, but returns
// early with ctx.Err() if the context is cancelled first. A non-positive
// duration returns immediately (after checking ctx).
func sleepWithContext(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d <= 0 {
		return nil
	}

	done := make(chan struct{})
	go func() {
		sleepFunc(d)
		close(done)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}
