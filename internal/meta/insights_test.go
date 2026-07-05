package meta

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// loadFixture reads a JSON fixture from the testdata directory.
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

// TestFetchTodayInsights_Pagination asserts FetchTodayInsights follows the
// paging.next cursor across two pages, combining rows into a single slice,
// and correctly parses nested actions/purchase_roas arrays.
func TestFetchTodayInsights_Pagination(t *testing.T) {
	page1 := loadFixture(t, "insights_paginated_page1.json")
	page2 := loadFixture(t, "insights_paginated_page2.json")

	var page2Path = "/v21.0/act_123/insights/page2"

	mux := http.NewServeMux()
	var page1URL string
	mux.HandleFunc("/act_123/insights", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("level"); got != "campaign" {
			t.Errorf("level = %q, want campaign", got)
		}
		if got := r.URL.Query().Get("date_preset"); got != "today" {
			t.Errorf("date_preset = %q, want today", got)
		}
		if got := r.URL.Query().Get("access_token"); got != "test-token" {
			t.Errorf("access_token = %q, want test-token", got)
		}
		// Rewrite the fixture's placeholder next URL to point at page 2 on
		// this test server.
		body := strings.Replace(string(page1), "__NEXT_URL__", page1URL, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})
	mux.HandleFunc(page2Path, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(page2)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()
	page1URL = srv.URL + page2Path

	c := NewClient("test-token")
	c.baseURL = srv.URL

	rows, err := FetchTodayInsights(context.Background(), c, "123")
	if err != nil {
		t.Fatalf("FetchTodayInsights returned error: %v", err)
	}

	// Combined: 2 rows from page 1 + 1 row from page 2 = 3.
	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3", len(rows))
	}

	// Assert at least one row has a non-empty actions array parsed correctly.
	var withActions *InsightRow
	for i := range rows {
		if len(rows[i].Actions) > 0 {
			withActions = &rows[i]
			break
		}
	}
	if withActions == nil {
		t.Fatal("no row with non-empty actions array; want at least one")
	}

	// Verify nested action parsing: page 1 row 0 has a purchase action with value "42".
	first := rows[0]
	if first.CampaignID != "23851234500010001" {
		t.Errorf("rows[0].CampaignID = %q, want 23851234500010001", first.CampaignID)
	}
	if len(first.Actions) == 0 {
		t.Fatal("rows[0].Actions is empty; want non-empty")
	}
	var foundPurchase bool
	for _, a := range first.Actions {
		if a.ActionType == "purchase" && a.Value == "42" {
			foundPurchase = true
			break
		}
	}
	if !foundPurchase {
		t.Errorf("rows[0].Actions missing purchase=42; got %+v", first.Actions)
	}

	// Verify purchase_roas parses.
	if len(first.PurchaseRoas) == 0 || first.PurchaseRoas[0].Value != "4.25" {
		t.Errorf("rows[0].PurchaseRoas = %+v, want first value 4.25", first.PurchaseRoas)
	}

	// Verify page 2 row survived.
	last := rows[2]
	if last.CampaignID != "23851234500010003" {
		t.Errorf("rows[2].CampaignID = %q, want 23851234500010003", last.CampaignID)
	}
}

// TestFetchTodayInsights_EmptyResult asserts an empty data array yields an
// empty slice, not a nil-panic.
func TestFetchTodayInsights_EmptyResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"paging":{}}`))
	}))
	defer srv.Close()

	c := NewClient("test-token")
	c.baseURL = srv.URL

	rows, err := FetchTodayInsights(context.Background(), c, "123")
	if err != nil {
		t.Fatalf("FetchTodayInsights returned error: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("len(rows) = %d, want 0", len(rows))
	}
}

// TestListCampaigns_Pagination asserts ListCampaigns follows paging.next.
func TestListCampaigns_Pagination(t *testing.T) {
	var page2Path = "/v21.0/act_123/campaigns/page2"

	mux := http.NewServeMux()
	var page1NextURL string
	mux.HandleFunc("/act_123/campaigns", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("fields"); got != "id,name" {
			t.Errorf("fields = %q, want id,name", got)
		}
		resp := map[string]any{
			"data": []Campaign{
				{ID: "1", Name: "Alpha"},
				{ID: "2", Name: "Beta"},
			},
			"paging": map[string]any{
				"next": page1NextURL,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc(page2Path, func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": []Campaign{
				{ID: "3", Name: "Gamma"},
			},
			"paging": map[string]any{},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()
	page1NextURL = srv.URL + page2Path

	c := NewClient("test-token")
	c.baseURL = srv.URL

	campaigns, err := ListCampaigns(context.Background(), c, "123")
	if err != nil {
		t.Fatalf("ListCampaigns returned error: %v", err)
	}
	if len(campaigns) != 3 {
		t.Fatalf("len(campaigns) = %d, want 3", len(campaigns))
	}
	if campaigns[0].Name != "Alpha" || campaigns[2].Name != "Gamma" {
		t.Errorf("unexpected campaign order: %+v", campaigns)
	}
}
