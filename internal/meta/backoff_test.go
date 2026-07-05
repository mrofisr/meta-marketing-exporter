package meta

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestIsThrottled_ThrottleCodes(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		wantBool bool
	}{
		{"code 4 - app limit", 4, true},
		{"code 17 - user limit", 17, true},
		{"code 613 - rate limit", 613, true},
		{"code 80000 - biz use case", 80000, true},
		{"code 80001 - biz use case", 80001, true},
		{"code 80002 - biz use case", 80002, true},
		{"code 80003 - biz use case", 80003, true},
		{"code 80004 - biz use case", 80004, true},
		{"code 190 - auth expired", 190, false},
		{"code 102 - session expired", 102, false},
		{"code 100 - invalid params", 100, false},
		{"code 0 - zero", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &MetaAPIError{Code: tt.code, Message: "test"}
			gotBool, gotDur := IsThrottled(err)
			if gotBool != tt.wantBool {
				t.Errorf("IsThrottled(code=%d) = %v, want %v", tt.code, gotBool, tt.wantBool)
			}
			if tt.wantBool && gotDur != defaultThrottleDelay {
				t.Errorf("IsThrottled(code=%d) duration = %v, want %v", tt.code, gotDur, defaultThrottleDelay)
			}
			if !tt.wantBool && gotDur != 0 {
				t.Errorf("IsThrottled(code=%d) duration = %v, want 0", tt.code, gotDur)
			}
		})
	}
}

func TestIsThrottled_NilAndNonAPIError(t *testing.T) {
	// nil error
	gotBool, gotDur := IsThrottled(nil)
	if gotBool || gotDur != 0 {
		t.Errorf("IsThrottled(nil) = (%v, %v), want (false, 0)", gotBool, gotDur)
	}

	// non-MetaAPIError
	gotBool, gotDur = IsThrottled(errors.New("network timeout"))
	if gotBool || gotDur != 0 {
		t.Errorf("IsThrottled(generic error) = (%v, %v), want (false, 0)", gotBool, gotDur)
	}
}

func TestIsThrottled_WrappedError(t *testing.T) {
	inner := &MetaAPIError{Code: 17, Message: "rate limited"}
	wrapped := fmt.Errorf("fetch insights: %w", inner)

	gotBool, gotDur := IsThrottled(wrapped)
	if !gotBool {
		t.Error("IsThrottled should detect wrapped MetaAPIError with throttle code")
	}
	if gotDur != defaultThrottleDelay {
		t.Errorf("expected delay %v, got %v", defaultThrottleDelay, gotDur)
	}
}

func TestParseRetryHeader_Valid(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Business-Use-Case-Usage",
		`{"act_123456":[{"type":"ads_management","estimated_time_to_regain_access":15}]}`)

	dur, ok := ParseRetryHeader(headers)
	if !ok {
		t.Fatal("ParseRetryHeader returned false for valid header")
	}
	if dur != 15*time.Minute {
		t.Errorf("ParseRetryHeader = %v, want %v", dur, 15*time.Minute)
	}
}

func TestParseRetryHeader_MultipleEntries(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Business-Use-Case-Usage",
		`{"act_111":[{"type":"ads_insights","estimated_time_to_regain_access":5},{"type":"ads_management","estimated_time_to_regain_access":20}],"act_222":[{"type":"ads_insights","estimated_time_to_regain_access":10}]}`)

	dur, ok := ParseRetryHeader(headers)
	if !ok {
		t.Fatal("ParseRetryHeader returned false")
	}
	// Should pick the largest: 20 minutes
	if dur != 20*time.Minute {
		t.Errorf("ParseRetryHeader = %v, want %v", dur, 20*time.Minute)
	}
}

func TestParseRetryHeader_Absent(t *testing.T) {
	headers := http.Header{}
	dur, ok := ParseRetryHeader(headers)
	if ok || dur != 0 {
		t.Errorf("expected (0, false) for absent header, got (%v, %v)", dur, ok)
	}
}

func TestParseRetryHeader_NilHeaders(t *testing.T) {
	dur, ok := ParseRetryHeader(nil)
	if ok || dur != 0 {
		t.Errorf("expected (0, false) for nil headers, got (%v, %v)", dur, ok)
	}
}

func TestParseRetryHeader_MalformedJSON(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Business-Use-Case-Usage", `not valid json`)
	dur, ok := ParseRetryHeader(headers)
	if ok || dur != 0 {
		t.Errorf("expected (0, false) for malformed JSON, got (%v, %v)", dur, ok)
	}
}

func TestParseRetryHeader_ZeroMinutes(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Business-Use-Case-Usage",
		`{"act_123":[{"type":"ads_insights","estimated_time_to_regain_access":0}]}`)
	dur, ok := ParseRetryHeader(headers)
	if ok || dur != 0 {
		t.Errorf("expected (0, false) for zero minutes, got (%v, %v)", dur, ok)
	}
}

func TestWithBackoff_SuccessOnFirstCall(t *testing.T) {
	overrideSleep(t)

	var calls int32
	err := WithBackoff(context.Background(), func() error {
		atomic.AddInt32(&calls, 1)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("fn called %d times, want 1", atomic.LoadInt32(&calls))
	}
}

func TestWithBackoff_RetriesOnThrottle(t *testing.T) {
	overrideSleep(t)

	var calls int32
	throttleErr := &MetaAPIError{Code: 17, Message: "rate limited"}

	err := WithBackoff(context.Background(), func() error {
		n := atomic.AddInt32(&calls, 1)
		if n <= 2 {
			return throttleErr
		}
		return nil // succeed on 3rd call
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Errorf("fn called %d times, want 3 (2 retries then success)", atomic.LoadInt32(&calls))
	}
}

func TestWithBackoff_ExhaustsThrottleRetries(t *testing.T) {
	overrideSleep(t)

	var calls int32
	throttleErr := &MetaAPIError{Code: 80000, Message: "biz rate limit"}

	err := WithBackoff(context.Background(), func() error {
		atomic.AddInt32(&calls, 1)
		return throttleErr
	})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	// 1 initial + 5 retries = 6 calls
	if atomic.LoadInt32(&calls) != throttleMaxRetries+1 {
		t.Errorf("fn called %d times, want %d", atomic.LoadInt32(&calls), throttleMaxRetries+1)
	}
}

func TestWithBackoff_NoRetryOnAuthError(t *testing.T) {
	overrideSleep(t)

	var calls int32
	authErr := &MetaAPIError{Code: 190, Message: "token expired"}

	err := WithBackoff(context.Background(), func() error {
		atomic.AddInt32(&calls, 1)
		return authErr
	})
	if err == nil {
		t.Fatal("expected auth error to be returned")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("fn called %d times, want 1 (no retry for auth)", atomic.LoadInt32(&calls))
	}
	var gotErr *MetaAPIError
	if !errors.As(err, &gotErr) || gotErr.Code != 190 {
		t.Errorf("expected MetaAPIError code 190, got: %v", err)
	}
}

func TestWithBackoff_NoRetryOnAuthError102(t *testing.T) {
	overrideSleep(t)

	var calls int32
	authErr := &MetaAPIError{Code: 102, Message: "session expired"}

	err := WithBackoff(context.Background(), func() error {
		atomic.AddInt32(&calls, 1)
		return authErr
	})
	if err == nil {
		t.Fatal("expected auth error to be returned")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("fn called %d times, want 1 (no retry for auth 102)", atomic.LoadInt32(&calls))
	}
}

func TestWithBackoff_RetriesTransientError(t *testing.T) {
	overrideSleep(t)

	var calls int32
	transientErr := errors.New("network timeout")

	err := WithBackoff(context.Background(), func() error {
		n := atomic.AddInt32(&calls, 1)
		if n <= 2 {
			return transientErr
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Errorf("fn called %d times, want 3", atomic.LoadInt32(&calls))
	}
}

func TestWithBackoff_ExhaustsTransientRetries(t *testing.T) {
	overrideSleep(t)

	var calls int32
	transientErr := errors.New("connection refused")

	err := WithBackoff(context.Background(), func() error {
		atomic.AddInt32(&calls, 1)
		return transientErr
	})
	if err == nil {
		t.Fatal("expected error after exhausting transient retries")
	}
	// 1 initial + 3 retries = 4 calls
	if atomic.LoadInt32(&calls) != transientMaxRetries+1 {
		t.Errorf("fn called %d times, want %d", atomic.LoadInt32(&calls), transientMaxRetries+1)
	}
}

func TestWithBackoff_RespectsContextCancellation(t *testing.T) {
	overrideSleep(t)

	ctx, cancel := context.WithCancel(context.Background())
	var calls int32
	throttleErr := &MetaAPIError{Code: 17, Message: "rate limited"}

	// Cancel after first call
	err := WithBackoff(ctx, func() error {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			cancel()
		}
		return throttleErr
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

// overrideSleep replaces the package-level sleepFunc with a no-op for the
// duration of the test, restoring it via t.Cleanup.
func overrideSleep(t *testing.T) {
	t.Helper()
	orig := sleepFunc
	sleepFunc = func(time.Duration) {} // no-op: tests don't actually wait
	t.Cleanup(func() { sleepFunc = orig })
}
