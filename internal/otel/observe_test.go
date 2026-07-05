package otel_test

import (
	"context"
	"testing"

	"meta-marketing-exporter/internal/meta"
	"meta-marketing-exporter/internal/otel"

	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

func collect(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	return rm
}

func gaugePoints(t *testing.T, rm metricdata.ResourceMetrics, name string) []metricdata.DataPoint[float64] {
	t.Helper()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			g, ok := m.Data.(metricdata.Gauge[float64])
			if !ok {
				t.Fatalf("metric %q is not a Float64 gauge", name)
			}
			return g.DataPoints
		}
	}
	return nil
}

func setupMeter(t *testing.T) (*sdkmetric.ManualReader, *otel.GaugeInstruments, *otel.MetricsStore) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	meter := provider.Meter("test")
	gauges, err := otel.NewGaugeInstruments(meter)
	if err != nil {
		t.Fatalf("NewGaugeInstruments: %v", err)
	}

	store := otel.NewMetricsStore()
	reg, err := otel.RegisterGaugeCallbacks(meter, gauges, store)
	if err != nil {
		t.Fatalf("RegisterGaugeCallbacks: %v", err)
	}
	t.Cleanup(func() { _ = reg.Unregister() })

	return reader, gauges, store
}

func TestRegisterGaugeCallbacks_EmitsMetrics(t *testing.T) {
	reader, _, store := setupMeter(t)

	store.Update([]meta.CampaignMetrics{
		{
			CampaignID:      "c1",
			CampaignName:    "Alpha",
			AccountCurrency: "USD",
			Spend:           100,
			Conversions:     5,
			ConversionRate:  0.1,
		},
	})

	rm := collect(t, reader)

	spend := gaugePoints(t, rm, "meta_ads.spend")
	if len(spend) != 1 {
		t.Fatalf("meta_ads.spend datapoints = %d, want 1", len(spend))
	}
	if spend[0].Value != 100 {
		t.Errorf("spend value = %v, want 100", spend[0].Value)
	}

	cid, ok := spend[0].Attributes.Value("campaign_id")
	if !ok || cid.AsString() != "c1" {
		t.Errorf("campaign_id attribute = %v (present=%v), want c1", cid.AsString(), ok)
	}
	name, ok := spend[0].Attributes.Value("campaign_name")
	if !ok || name.AsString() != "Alpha" {
		t.Errorf("campaign_name attribute = %v, want Alpha", name.AsString())
	}
	cur, ok := spend[0].Attributes.Value("currency")
	if !ok || cur.AsString() != "USD" {
		t.Errorf("currency attribute = %v, want USD", cur.AsString())
	}

	conv := gaugePoints(t, rm, "meta_ads.conversions")
	if len(conv) != 1 || conv[0].Value != 5 {
		t.Errorf("conversions = %+v, want single value 5", conv)
	}
}

func TestRegisterGaugeCallbacks_EmptyStoreNoEmission(t *testing.T) {
	reader, _, _ := setupMeter(t)

	rm := collect(t, reader)

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			g, ok := m.Data.(metricdata.Gauge[float64])
			if ok && len(g.DataPoints) > 0 {
				t.Errorf("metric %q emitted %d datapoints with empty store, want 0",
					m.Name, len(g.DataPoints))
			}
		}
	}
}

func TestRegisterGaugeCallbacks_AllZeroDeliveryEmitted(t *testing.T) {
	reader, _, store := setupMeter(t)

	store.Update([]meta.CampaignMetrics{
		{CampaignID: "zero", CampaignName: "NoDelivery", AccountCurrency: "EUR"},
	})

	rm := collect(t, reader)

	spend := gaugePoints(t, rm, "meta_ads.spend")
	if len(spend) != 1 {
		t.Fatalf("meta_ads.spend datapoints = %d, want 1 (zero delivery still emits)", len(spend))
	}
	if spend[0].Value != 0 {
		t.Errorf("spend value = %v, want 0", spend[0].Value)
	}
}

func TestRegisterGaugeCallbacks_MultipleCampaigns(t *testing.T) {
	reader, _, store := setupMeter(t)

	store.Update([]meta.CampaignMetrics{
		{CampaignID: "c1", Spend: 10},
		{CampaignID: "c2", Spend: 20},
	})

	rm := collect(t, reader)

	spend := gaugePoints(t, rm, "meta_ads.spend")
	if len(spend) != 2 {
		t.Fatalf("meta_ads.spend datapoints = %d, want 2", len(spend))
	}
}
