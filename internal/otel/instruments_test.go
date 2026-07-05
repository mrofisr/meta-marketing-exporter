package otel_test

import (
	"testing"

	"meta-marketing-exporter/internal/otel"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

func TestNewGaugeInstruments_RegistersAll12(t *testing.T) {
	provider := sdkmetric.NewMeterProvider()
	defer func() { _ = provider.Shutdown(nil) }()

	meter := provider.Meter("test")

	instruments, err := otel.NewGaugeInstruments(meter)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if instruments == nil {
		t.Fatal("expected non-nil instruments")
	}

	// Verify all 12 fields are non-nil
	checks := []struct {
		name  string
		gauge interface{}
	}{
		{"Spend", instruments.Spend},
		{"Impressions", instruments.Impressions},
		{"Clicks", instruments.Clicks},
		{"CTR", instruments.CTR},
		{"CPM", instruments.CPM},
		{"CPC", instruments.CPC},
		{"Reach", instruments.Reach},
		{"Frequency", instruments.Frequency},
		{"ROAS", instruments.ROAS},
		{"Conversions", instruments.Conversions},
		{"ConversionRate", instruments.ConversionRate},
		{"CostPerConversion", instruments.CostPerConversion},
	}

	for _, c := range checks {
		if c.gauge == nil {
			t.Errorf("expected %s gauge to be non-nil", c.name)
		}
	}

	if len(checks) != 12 {
		t.Errorf("expected 12 gauge instruments, got %d", len(checks))
	}
}
