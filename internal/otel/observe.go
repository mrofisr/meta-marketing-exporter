package otel

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"meta-marketing-exporter/internal/meta"
)

// MetricsStore holds the current campaign metrics and provides thread-safe
// access for the OTel callback. Call Update() after each fetch cycle; the
// registered gauge callbacks will observe these values on the next scrape.
type MetricsStore struct {
	mu      sync.RWMutex
	metrics []meta.CampaignMetrics
}

// NewMetricsStore returns an empty store ready for Update calls.
func NewMetricsStore() *MetricsStore {
	return &MetricsStore{}
}

// Update replaces the stored metrics slice. Safe for concurrent use with
// gauge callbacks reading from the store.
func (s *MetricsStore) Update(metrics []meta.CampaignMetrics) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metrics = metrics
}

// snapshot returns a copy of current metrics for safe iteration.
func (s *MetricsStore) snapshot() []meta.CampaignMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]meta.CampaignMetrics, len(s.metrics))
	copy(out, s.metrics)
	return out
}

// RegisterGaugeCallbacks registers callbacks for the 12 gauge instruments on
// the given meter. Each callback iterates the current MetricsStore snapshot
// and emits one observation per campaign with campaign_id, campaign_name,
// currency, and date range as attributes.
//
// If the store is empty (no campaigns), no observations are emitted - this
// leaves gauges stale rather than zeroing them (Decision 9: dropout behavior).
// If a campaign has all-zero delivery, zero values ARE emitted (not skipped).
//
// Returns the registration handle so callers can Unregister() on shutdown.
func RegisterGaugeCallbacks(meter metric.Meter, gauges *GaugeInstruments, store *MetricsStore) (metric.Registration, error) {
	return meter.RegisterCallback(
		func(_ context.Context, o metric.Observer) error {
			metrics := store.snapshot()
			for _, m := range metrics {
				attrs := campaignAttributes(m)
				opt := metric.WithAttributes(attrs...)
				o.ObserveFloat64(gauges.Spend, m.Spend, opt)
				o.ObserveFloat64(gauges.Impressions, m.Impressions, opt)
				o.ObserveFloat64(gauges.Clicks, m.Clicks, opt)
				o.ObserveFloat64(gauges.CTR, m.CTR, opt)
				o.ObserveFloat64(gauges.CPM, m.CPM, opt)
				o.ObserveFloat64(gauges.CPC, m.CPC, opt)
				o.ObserveFloat64(gauges.Reach, m.Reach, opt)
				o.ObserveFloat64(gauges.Frequency, m.Frequency, opt)
				o.ObserveFloat64(gauges.ROAS, m.Roas, opt)
				o.ObserveFloat64(gauges.Conversions, m.Conversions, opt)
				o.ObserveFloat64(gauges.ConversionRate, m.ConversionRate, opt)
				o.ObserveFloat64(gauges.CostPerConversion, m.CostPerConversion, opt)
			}
			return nil
		},
		gauges.Spend,
		gauges.Impressions,
		gauges.Clicks,
		gauges.CTR,
		gauges.CPM,
		gauges.CPC,
		gauges.Reach,
		gauges.Frequency,
		gauges.ROAS,
		gauges.Conversions,
		gauges.ConversionRate,
		gauges.CostPerConversion,
	)
}

// campaignAttributes returns the standard attribute set for a campaign metric
// observation. These are labels, not gauge values (Decision 4).
func campaignAttributes(m meta.CampaignMetrics) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("campaign_id", m.CampaignID),
		attribute.String("campaign_name", m.CampaignName),
		attribute.String("currency", m.AccountCurrency),
		attribute.String("date_start", m.DateStart),
		attribute.String("date_stop", m.DateStop),
	}
}
