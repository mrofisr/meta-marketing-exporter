package meta

import (
	"log/slog"
	"strconv"
)

// purchaseActionTypes enumerates the Meta action_type values that count as
// conversions for this exporter (Decision 8). Extend cautiously - adding
// entries here retroactively changes historical time-series shape.
var purchaseActionTypes = map[string]struct{}{
	"purchase":                             {},
	"offsite_conversion.fb_pixel_purchase": {},
}

// CampaignMetrics is the flat, numeric view of a single InsightRow ready to
// be observed as OpenTelemetry gauges. String fields at the tail are used
// as metric attributes (labels), never as gauge values.
type CampaignMetrics struct {
	// Numeric gauge values (Decision 4).
	Spend             float64
	Impressions       float64
	Clicks            float64
	CTR               float64
	CPM               float64
	CPC               float64
	Reach             float64
	Frequency         float64
	Roas              float64
	Conversions       float64
	ConversionRate    float64
	CostPerConversion float64

	// Attribute (label) values.
	CampaignID      string
	CampaignName    string
	AccountCurrency string
	DateStart       string
	DateStop        string
}

// ExtractMetrics converts a Meta Insights row into the flat gauge-friendly
// CampaignMetrics shape. It never panics on malformed data - unparseable
// numeric strings default to 0 and are logged at warn level. Divide-by-zero
// on derived rates yields 0 rather than NaN or panic.
func ExtractMetrics(row InsightRow) CampaignMetrics {
	m := CampaignMetrics{
		CampaignID:      row.CampaignID,
		CampaignName:    row.CampaignName,
		AccountCurrency: row.AccountCurrency,
		DateStart:       row.DateStart,
		DateStop:        row.DateStop,
	}

	m.Spend = parseFloat(row.Spend, "spend", row.CampaignID)
	m.Impressions = parseFloat(row.Impressions, "impressions", row.CampaignID)
	m.Clicks = parseFloat(row.Clicks, "clicks", row.CampaignID)
	m.CTR = parseFloat(row.CTR, "ctr", row.CampaignID)
	m.CPM = parseFloat(row.CPM, "cpm", row.CampaignID)
	m.CPC = parseFloat(row.CPC, "cpc", row.CampaignID)
	m.Reach = parseFloat(row.Reach, "reach", row.CampaignID)
	m.Frequency = parseFloat(row.Frequency, "frequency", row.CampaignID)

	m.Conversions = sumActions(row.Actions, purchaseActionTypes, "conversions", row.CampaignID)
	m.Roas = sumRoas(row.PurchaseRoas, row.CampaignID)

	if m.Clicks > 0 {
		m.ConversionRate = m.Conversions / m.Clicks
	}
	if m.Conversions > 0 {
		m.CostPerConversion = m.Spend / m.Conversions
	}

	return m
}

// parseFloat parses a Meta API numeric-string field, returning 0 and logging
// a warning on parse failure. Empty strings map to 0 without warning (Meta
// omits fields for campaigns with no delivery, which is a normal state).
func parseFloat(s, field, campaignID string) float64 {
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		slog.Warn("meta: malformed numeric field",
			"field", field,
			"campaign_id", campaignID,
			"value", s,
			"error", err,
		)
		return 0
	}
	return v
}

// sumActions sums the Value fields across an action array, restricted to
// entries whose ActionType is present in the allowed set.
func sumActions(actions []Action, allowed map[string]struct{}, field, campaignID string) float64 {
	var total float64
	for _, a := range actions {
		if _, ok := allowed[a.ActionType]; !ok {
			continue
		}
		total += parseFloat(a.Value, field+"["+a.ActionType+"]", campaignID)
	}
	return total
}

// sumRoas sums the purchase_roas array. Meta returns purchase_roas as an
// action-shaped array where each entry's ActionType is a subtype and Value
// is the ROAS multiple; the exporter sums all entries because a campaign
// with a single conversion pixel emits one entry, and there is no upstream
// filter needed here.
func sumRoas(roas []Action, campaignID string) float64 {
	var total float64
	for _, a := range roas {
		total += parseFloat(a.Value, "purchase_roas["+a.ActionType+"]", campaignID)
	}
	return total
}
