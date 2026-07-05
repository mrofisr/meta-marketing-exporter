package meta

import (
	"math"
	"testing"
)

func TestExtractMetrics(t *testing.T) {
	tests := []struct {
		name   string
		row    InsightRow
		checks func(t *testing.T, m CampaignMetrics)
	}{
		{
			name: "action_filtering_purchase_only",
			row: InsightRow{
				CampaignID:   "123",
				CampaignName: "Test Campaign",
				Spend:        "100.50",
				Clicks:       "200",
				Actions: []Action{
					{ActionType: "purchase", Value: "5"},
					{ActionType: "link_click", Value: "100"},
					{ActionType: "offsite_conversion.fb_pixel_purchase", Value: "3"},
				},
			},
			checks: func(t *testing.T, m CampaignMetrics) {
				if m.Conversions != 8 {
					t.Errorf("Conversions = %v, want 8 (5 purchase + 3 fb_pixel_purchase)", m.Conversions)
				}
				if m.Spend != 100.50 {
					t.Errorf("Spend = %v, want 100.50", m.Spend)
				}
			},
		},
		{
			name: "link_click_excluded_from_conversions",
			row: InsightRow{
				CampaignID: "456",
				Clicks:     "50",
				Actions: []Action{
					{ActionType: "purchase", Value: "5"},
					{ActionType: "link_click", Value: "100"},
				},
			},
			checks: func(t *testing.T, m CampaignMetrics) {
				if m.Conversions != 5 {
					t.Errorf("Conversions = %v, want 5 (link_click should be excluded)", m.Conversions)
				}
			},
		},
		{
			name: "zero_clicks_no_panic_on_conversion_rate",
			row: InsightRow{
				CampaignID: "789",
				Clicks:     "0",
				Actions: []Action{
					{ActionType: "purchase", Value: "10"},
				},
			},
			checks: func(t *testing.T, m CampaignMetrics) {
				if m.Clicks != 0 {
					t.Errorf("Clicks = %v, want 0", m.Clicks)
				}
				if m.ConversionRate != 0 {
					t.Errorf("ConversionRate = %v, want 0 (no divide-by-zero panic)", m.ConversionRate)
				}
			},
		},
		{
			name: "zero_conversions_no_panic_on_cost_per_conversion",
			row: InsightRow{
				CampaignID: "101",
				Spend:      "500.00",
				Actions:    []Action{},
			},
			checks: func(t *testing.T, m CampaignMetrics) {
				if m.Conversions != 0 {
					t.Errorf("Conversions = %v, want 0", m.Conversions)
				}
				if m.CostPerConversion != 0 {
					t.Errorf("CostPerConversion = %v, want 0 (no divide-by-zero panic)", m.CostPerConversion)
				}
			},
		},
		{
			name: "empty_row_yields_zero_values",
			row:  InsightRow{CampaignID: "empty"},
			checks: func(t *testing.T, m CampaignMetrics) {
				if m.Spend != 0 || m.Impressions != 0 || m.Clicks != 0 ||
					m.CTR != 0 || m.CPM != 0 || m.CPC != 0 ||
					m.Reach != 0 || m.Frequency != 0 || m.Roas != 0 ||
					m.Conversions != 0 || m.ConversionRate != 0 || m.CostPerConversion != 0 {
					t.Errorf("empty row should yield all zeros, got %+v", m)
				}
				if m.CampaignID != "empty" {
					t.Errorf("CampaignID = %q, want empty", m.CampaignID)
				}
			},
		},
		{
			name: "malformed_numeric_strings_default_to_zero",
			row: InsightRow{
				CampaignID:  "bad",
				Spend:       "not-a-number",
				Impressions: "12.34.56",
				Clicks:      "100",
			},
			checks: func(t *testing.T, m CampaignMetrics) {
				if m.Spend != 0 {
					t.Errorf("malformed Spend = %v, want 0", m.Spend)
				}
				if m.Impressions != 0 {
					t.Errorf("malformed Impressions = %v, want 0", m.Impressions)
				}
				if m.Clicks != 100 {
					t.Errorf("valid Clicks = %v, want 100", m.Clicks)
				}
			},
		},
		{
			name: "purchase_roas_summed",
			row: InsightRow{
				CampaignID: "roas",
				PurchaseRoas: []Action{
					{ActionType: "action:omni_purchase", Value: "2.5"},
					{ActionType: "action:onsite_web_purchase", Value: "1.5"},
				},
			},
			checks: func(t *testing.T, m CampaignMetrics) {
				if m.Roas != 4.0 {
					t.Errorf("Roas = %v, want 4.0 (2.5 + 1.5)", m.Roas)
				}
			},
		},
		{
			name: "derived_fields_calculated_correctly",
			row: InsightRow{
				CampaignID: "derived",
				Spend:      "100",
				Clicks:     "50",
				Actions: []Action{
					{ActionType: "purchase", Value: "10"},
				},
			},
			checks: func(t *testing.T, m CampaignMetrics) {
				expectedCR := 10.0 / 50.0
				if math.Abs(m.ConversionRate-expectedCR) > 0.0001 {
					t.Errorf("ConversionRate = %v, want %v", m.ConversionRate, expectedCR)
				}
				expectedCPC := 100.0 / 10.0
				if math.Abs(m.CostPerConversion-expectedCPC) > 0.0001 {
					t.Errorf("CostPerConversion = %v, want %v", m.CostPerConversion, expectedCPC)
				}
			},
		},
		{
			name: "labels_copied_correctly",
			row: InsightRow{
				CampaignID:      "label-test",
				CampaignName:    "My Campaign",
				AccountCurrency: "USD",
				DateStart:       "2026-07-05",
				DateStop:        "2026-07-05",
			},
			checks: func(t *testing.T, m CampaignMetrics) {
				if m.CampaignID != "label-test" {
					t.Errorf("CampaignID = %q, want label-test", m.CampaignID)
				}
				if m.CampaignName != "My Campaign" {
					t.Errorf("CampaignName = %q, want My Campaign", m.CampaignName)
				}
				if m.AccountCurrency != "USD" {
					t.Errorf("AccountCurrency = %q, want USD", m.AccountCurrency)
				}
				if m.DateStart != "2026-07-05" {
					t.Errorf("DateStart = %q, want 2026-07-05", m.DateStart)
				}
				if m.DateStop != "2026-07-05" {
					t.Errorf("DateStop = %q, want 2026-07-05", m.DateStop)
				}
			},
		},
		{
			name: "all_simple_fields_parsed",
			row: InsightRow{
				CampaignID:  "full",
				Spend:       "123.45",
				Impressions: "10000",
				Clicks:      "500",
				CTR:         "5.0",
				CPM:         "12.345",
				CPC:         "0.2469",
				Reach:       "8000",
				Frequency:   "1.25",
			},
			checks: func(t *testing.T, m CampaignMetrics) {
				if m.Spend != 123.45 {
					t.Errorf("Spend = %v, want 123.45", m.Spend)
				}
				if m.Impressions != 10000 {
					t.Errorf("Impressions = %v, want 10000", m.Impressions)
				}
				if m.Clicks != 500 {
					t.Errorf("Clicks = %v, want 500", m.Clicks)
				}
				if m.CTR != 5.0 {
					t.Errorf("CTR = %v, want 5.0", m.CTR)
				}
				if m.CPM != 12.345 {
					t.Errorf("CPM = %v, want 12.345", m.CPM)
				}
				if math.Abs(m.CPC-0.2469) > 0.0001 {
					t.Errorf("CPC = %v, want 0.2469", m.CPC)
				}
				if m.Reach != 8000 {
					t.Errorf("Reach = %v, want 8000", m.Reach)
				}
				if m.Frequency != 1.25 {
					t.Errorf("Frequency = %v, want 1.25", m.Frequency)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := ExtractMetrics(tt.row)
			tt.checks(t, m)
		})
	}
}

// TestExtractMetrics_EmptyDeliveryEmitsZeros locks Decision 9: a campaign that
// is reported by Meta but had zero delivery today (labels present, numeric
// fields absent/empty) must yield all-zero numeric gauges - NOT be dropped and
// NOT panic. This is distinct from campaign dropout (absent from the response),
// which is handled at the observe/store layer by leaving gauges stale.
func TestExtractMetrics_EmptyDeliveryEmitsZeros(t *testing.T) {
	row := InsightRow{
		CampaignID:      "act_no_delivery",
		CampaignName:    "Paused Campaign",
		AccountCurrency: "USD",
		DateStart:       "2026-07-05",
		DateStop:        "2026-07-05",
	}

	m := ExtractMetrics(row)

	numeric := map[string]float64{
		"Spend":             m.Spend,
		"Impressions":       m.Impressions,
		"Clicks":            m.Clicks,
		"CTR":               m.CTR,
		"CPM":               m.CPM,
		"CPC":               m.CPC,
		"Reach":             m.Reach,
		"Frequency":         m.Frequency,
		"Roas":              m.Roas,
		"Conversions":       m.Conversions,
		"ConversionRate":    m.ConversionRate,
		"CostPerConversion": m.CostPerConversion,
	}
	for name, v := range numeric {
		if v != 0 {
			t.Errorf("empty-delivery %s = %v, want 0 (Decision 9: emit 0, not drop)", name, v)
		}
	}

	if m.CampaignID != "act_no_delivery" {
		t.Errorf("CampaignID label lost: got %q", m.CampaignID)
	}
	if m.CampaignName != "Paused Campaign" {
		t.Errorf("CampaignName label lost: got %q", m.CampaignName)
	}
	if m.AccountCurrency != "USD" {
		t.Errorf("AccountCurrency label lost: got %q", m.AccountCurrency)
	}
}
