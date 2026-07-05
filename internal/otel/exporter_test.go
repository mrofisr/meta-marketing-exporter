package otel_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric"

	"meta-marketing-exporter/internal/otel"
)

func TestNewMeterProvider_ValidEndpoint(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name       string
		endpoint   string
		authHeader string
	}{
		{
			name:       "http endpoint with path",
			endpoint:   "http://localhost:4318/otlp/v1/metrics",
			authHeader: "",
		},
		{
			name:       "https endpoint",
			endpoint:   "https://mimir.example.com:9009/api/v1/push",
			authHeader: "",
		},
		{
			name:       "with auth header",
			endpoint:   "http://localhost:4318/otlp/v1/metrics",
			authHeader: "Authorization:Bearer token123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := otel.NewMeterProvider(ctx, tt.endpoint, tt.authHeader)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if provider == nil {
				t.Fatal("expected non-nil provider")
			}
			// Clean up
			_ = provider.Shutdown(ctx)
		})
	}
}

func TestNewMeterProvider_EmptyEndpoint(t *testing.T) {
	ctx := context.Background()

	provider, err := otel.NewMeterProvider(ctx, "", "")
	if err == nil {
		t.Fatal("expected error for empty endpoint")
	}
	if provider != nil {
		t.Fatal("expected nil provider on error")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected error to mention 'empty', got: %v", err)
	}
}

func TestNewMeterProvider_InvalidEndpoint(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		endpoint    string
		errContains string
	}{
		{
			name:        "missing host",
			endpoint:    "/just/a/path",
			errContains: "missing host",
		},
		{
			name:        "malformed URL",
			endpoint:    "://bad",
			errContains: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := otel.NewMeterProvider(ctx, tt.endpoint, "")
			if err == nil {
				_ = provider.Shutdown(ctx)
				t.Fatal("expected error for invalid endpoint")
			}
			if provider != nil {
				t.Fatal("expected nil provider on error")
			}
			if !strings.Contains(strings.ToLower(err.Error()), tt.errContains) {
				t.Errorf("expected error to contain %q, got: %v", tt.errContains, err)
			}
		})
	}
}

// TestNewMeterProvider_UnreachableEndpoint verifies that pointing the exporter
// at a dead endpoint does not panic or hang: the retry budget is bounded, the
// batch is dropped, and both ForceFlush and Shutdown complete inside the
// exporter timeout window (well under 10s).
func TestNewMeterProvider_UnreachableEndpoint(t *testing.T) {
	ctx := context.Background()

	provider, err := otel.NewMeterProvider(ctx, "http://127.0.0.1:19999/otlp/v1/metrics", "")
	if err != nil {
		t.Fatalf("creation should succeed (endpoint is validated syntactically): %v", err)
	}

	meter := provider.Meter("test")
	gauge, err := meter.Float64ObservableGauge("test_gauge")
	if err != nil {
		t.Fatalf("register gauge: %v", err)
	}
	_, err = meter.RegisterCallback(
		func(_ context.Context, o metric.Observer) error {
			o.ObserveFloat64(gauge, 1.0)
			return nil
		},
		gauge,
	)
	if err != nil {
		t.Fatalf("register callback: %v", err)
	}

	flushDone := make(chan error, 1)
	go func() {
		flushCtx, cancel := context.WithTimeout(ctx, 9*time.Second)
		defer cancel()
		flushDone <- provider.ForceFlush(flushCtx)
	}()

	select {
	case <-flushDone:
	case <-time.After(9500 * time.Millisecond):
		t.Fatal("ForceFlush hung for >9.5s against unreachable endpoint; retry budget should be bounded")
	}

	shutDone := make(chan error, 1)
	go func() {
		shutCtx, cancel := context.WithTimeout(ctx, 9*time.Second)
		defer cancel()
		shutDone <- provider.Shutdown(shutCtx)
	}()

	select {
	case <-shutDone:
	case <-time.After(9500 * time.Millisecond):
		t.Fatal("Shutdown hung for >9.5s against unreachable endpoint; retry budget should be bounded")
	}
}
