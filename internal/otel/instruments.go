package otel

import (
	"go.opentelemetry.io/otel/metric"
)

// GaugeInstruments holds the 12 observable gauge instruments for Meta Ads metrics.
type GaugeInstruments struct {
	Spend             metric.Float64ObservableGauge
	Impressions       metric.Float64ObservableGauge
	Clicks            metric.Float64ObservableGauge
	CTR               metric.Float64ObservableGauge
	CPM               metric.Float64ObservableGauge
	CPC               metric.Float64ObservableGauge
	Reach             metric.Float64ObservableGauge
	Frequency         metric.Float64ObservableGauge
	ROAS              metric.Float64ObservableGauge
	Conversions       metric.Float64ObservableGauge
	ConversionRate    metric.Float64ObservableGauge
	CostPerConversion metric.Float64ObservableGauge
}

// NewGaugeInstruments registers 12 Float64ObservableGauge instruments on the given meter.
func NewGaugeInstruments(meter metric.Meter) (*GaugeInstruments, error) {
	spend, err := meter.Float64ObservableGauge("meta_ads.spend")
	if err != nil {
		return nil, err
	}
	impressions, err := meter.Float64ObservableGauge("meta_ads.impressions")
	if err != nil {
		return nil, err
	}
	clicks, err := meter.Float64ObservableGauge("meta_ads.clicks")
	if err != nil {
		return nil, err
	}
	ctr, err := meter.Float64ObservableGauge("meta_ads.ctr")
	if err != nil {
		return nil, err
	}
	cpm, err := meter.Float64ObservableGauge("meta_ads.cpm")
	if err != nil {
		return nil, err
	}
	cpc, err := meter.Float64ObservableGauge("meta_ads.cpc")
	if err != nil {
		return nil, err
	}
	reach, err := meter.Float64ObservableGauge("meta_ads.reach")
	if err != nil {
		return nil, err
	}
	frequency, err := meter.Float64ObservableGauge("meta_ads.frequency")
	if err != nil {
		return nil, err
	}
	roas, err := meter.Float64ObservableGauge("meta_ads.roas")
	if err != nil {
		return nil, err
	}
	conversions, err := meter.Float64ObservableGauge("meta_ads.conversions")
	if err != nil {
		return nil, err
	}
	conversionRate, err := meter.Float64ObservableGauge("meta_ads.conversion_rate")
	if err != nil {
		return nil, err
	}
	costPerConversion, err := meter.Float64ObservableGauge("meta_ads.cost_per_conversion")
	if err != nil {
		return nil, err
	}

	return &GaugeInstruments{
		Spend:             spend,
		Impressions:       impressions,
		Clicks:            clicks,
		CTR:               ctr,
		CPM:               cpm,
		CPC:               cpc,
		Reach:             reach,
		Frequency:         frequency,
		ROAS:              roas,
		Conversions:       conversions,
		ConversionRate:    conversionRate,
		CostPerConversion: costPerConversion,
	}, nil
}
