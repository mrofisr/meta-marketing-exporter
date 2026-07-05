package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Campaign represents a Meta ad campaign with its ID and name.
type Campaign struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Action represents a single entry in Meta's actions, action_values, or
// purchase_roas arrays returned by the Insights API.
type Action struct {
	ActionType string `json:"action_type"`
	Value      string `json:"value"`
}

// InsightRow represents a single campaign-level insight row as returned by
// the Meta Marketing API Insights endpoint.
type InsightRow struct {
	CampaignID      string   `json:"campaign_id"`
	CampaignName    string   `json:"campaign_name"`
	Spend           string   `json:"spend"`
	AccountCurrency string   `json:"account_currency"`
	DateStart       string   `json:"date_start"`
	DateStop        string   `json:"date_stop"`
	Impressions     string   `json:"impressions"`
	Clicks          string   `json:"clicks"`
	CTR             string   `json:"ctr"`
	CPM             string   `json:"cpm"`
	CPC             string   `json:"cpc"`
	Reach           string   `json:"reach"`
	Frequency       string   `json:"frequency"`
	Actions         []Action `json:"actions"`
	ActionValues    []Action `json:"action_values"`
	PurchaseRoas    []Action `json:"purchase_roas"`
}

// paginatedResponse is the envelope Meta uses for paginated list endpoints.
type paginatedResponse struct {
	Data   json.RawMessage `json:"data"`
	Paging *paging         `json:"paging"`
}

type paging struct {
	Next string `json:"next"`
}

// ListCampaigns fetches all campaigns for the given ad account, following
// pagination cursors until exhausted.
func ListCampaigns(ctx context.Context, client *Client, accountID string) ([]Campaign, error) {
	params := url.Values{}
	params.Set("fields", "id,name")

	path := fmt.Sprintf("act_%s/campaigns", accountID)

	body, err := client.doRequest(ctx, http.MethodGet, path, params)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}

	var campaigns []Campaign
	nextURL, err := accumulatePage(body, &campaigns)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: parse page: %w", err)
	}

	for nextURL != "" {
		body, err = client.doFullURL(ctx, nextURL)
		if err != nil {
			return nil, fmt.Errorf("list campaigns: follow pagination: %w", err)
		}
		nextURL, err = accumulatePage(body, &campaigns)
		if err != nil {
			return nil, fmt.Errorf("list campaigns: parse page: %w", err)
		}
	}

	return campaigns, nil
}

// FetchTodayInsights fetches campaign-level insights for today for the given
// ad account, following pagination cursors until exhausted.
func FetchTodayInsights(ctx context.Context, client *Client, accountID string) ([]InsightRow, error) {
	params := url.Values{}
	params.Set("level", "campaign")
	params.Set("date_preset", "today")
	params.Set("fields", "campaign_id,campaign_name,spend,account_currency,date_start,date_stop,impressions,clicks,ctr,cpm,cpc,reach,frequency,actions,action_values,purchase_roas")

	path := fmt.Sprintf("act_%s/insights", accountID)

	body, err := client.doRequest(ctx, http.MethodGet, path, params)
	if err != nil {
		return nil, fmt.Errorf("fetch today insights: %w", err)
	}

	var rows []InsightRow
	nextURL, err := accumulatePage(body, &rows)
	if err != nil {
		return nil, fmt.Errorf("fetch today insights: parse page: %w", err)
	}

	for nextURL != "" {
		body, err = client.doFullURL(ctx, nextURL)
		if err != nil {
			return nil, fmt.Errorf("fetch today insights: follow pagination: %w", err)
		}
		nextURL, err = accumulatePage(body, &rows)
		if err != nil {
			return nil, fmt.Errorf("fetch today insights: parse page: %w", err)
		}
	}

	return rows, nil
}

// accumulatePage parses a paginated response body, appends items to dest, and
// returns the next page URL (empty string if no more pages).
func accumulatePage[T any](body []byte, dest *[]T) (string, error) {
	var page paginatedResponse
	if err := json.Unmarshal(body, &page); err != nil {
		return "", fmt.Errorf("unmarshal page envelope: %w", err)
	}

	var items []T
	if err := json.Unmarshal(page.Data, &items); err != nil {
		return "", fmt.Errorf("unmarshal page data: %w", err)
	}
	*dest = append(*dest, items...)

	if page.Paging != nil && page.Paging.Next != "" {
		return page.Paging.Next, nil
	}
	return "", nil
}

// doFullURL executes a GET against an absolute URL (used for pagination next
// links which Meta returns as complete URLs including access_token).
func (c *Client) doFullURL(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("meta: build pagination request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("meta: execute pagination request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("meta: read pagination response body: %w", err)
	}

	if apiErr := parseAPIError(body); apiErr != nil {
		return nil, apiErr
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &MetaAPIError{
			Code:    resp.StatusCode,
			Message: fmt.Sprintf("unexpected HTTP status %d", resp.StatusCode),
		}
	}

	return body, nil
}
