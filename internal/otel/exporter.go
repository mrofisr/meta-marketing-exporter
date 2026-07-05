// Package otel provides OpenTelemetry metric instrumentation for the Meta Ads exporter.
package otel

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Bounded so a stuck/unreachable OTLP endpoint cannot delay the next Meta poll
// cycle: WithTimeout caps the whole export (retries included), retryMaxElapsed
// caps backoff to ~3 attempts, both well under a poll interval.
const (
	exportTimeout    = 8 * time.Second
	retryInitialWait = 500 * time.Millisecond
	retryMaxWait     = 2 * time.Second
	retryMaxElapsed  = 6 * time.Second
)

// NewMeterProvider creates a configured OTel MeterProvider that exports metrics
// via OTLP/HTTP to the given endpoint. The endpoint should be a full URL like
// "http://localhost:4318/otlp/v1/metrics". If authHeader is non-empty, it must
// be in "Key:Value" format and will be sent with each export request.
func NewMeterProvider(ctx context.Context, endpoint string, authHeader string) (*sdkmetric.MeterProvider, error) {
	if endpoint == "" {
		return nil, errors.New("otel: endpoint must not be empty")
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, errors.New("otel: invalid endpoint URL: " + err.Error())
	}
	if parsed.Host == "" {
		return nil, errors.New("otel: endpoint missing host")
	}

	opts := []otlpmetrichttp.Option{
		otlpmetrichttp.WithEndpoint(parsed.Host),
		otlpmetrichttp.WithTimeout(exportTimeout),
		otlpmetrichttp.WithRetry(otlpmetrichttp.RetryConfig{
			Enabled:         true,
			InitialInterval: retryInitialWait,
			MaxInterval:     retryMaxWait,
			MaxElapsedTime:  retryMaxElapsed,
		}),
	}

	if parsed.Path != "" && parsed.Path != "/" {
		opts = append(opts, otlpmetrichttp.WithURLPath(parsed.Path))
	}

	if parsed.Scheme == "http" {
		opts = append(opts, otlpmetrichttp.WithInsecure())
	}

	if authHeader != "" {
		parts := strings.SplitN(authHeader, ":", 2)
		if len(parts) == 2 {
			opts = append(opts, otlpmetrichttp.WithHeaders(map[string]string{
				strings.TrimSpace(parts[0]): strings.TrimSpace(parts[1]),
			}))
		}
	}

	exporter, err := otlpmetrichttp.New(ctx, opts...)
	if err != nil {
		return nil, errors.New("otel: failed to create exporter: " + err.Error())
	}

	reader := sdkmetric.NewPeriodicReader(exporter)

	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		attribute.String(string(semconv.ServiceNameKey), "meta-ads-exporter"),
	)

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithResource(res),
	)

	return provider, nil
}
