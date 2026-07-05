package meta

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestClient_SuccessfulRequest asserts a 200 response with valid JSON is
// returned verbatim to the caller.
func TestClient_SuccessfulRequest(t *testing.T) {
	const wantName = "Campaign A"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("access_token"); got != "test-token" {
			t.Errorf("access_token param = %q, want %q", got, "test-token")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[{"id":"123","name":"Campaign A"}]}`))
	}))
	defer srv.Close()

	c := NewClient("test-token")
	c.baseURL = srv.URL

	body, err := c.doRequest(context.Background(), http.MethodGet, "/act_123/campaigns", nil)
	if err != nil {
		t.Fatalf("doRequest returned error: %v", err)
	}

	var parsed struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(parsed.Data) != 1 {
		t.Fatalf("len(data) = %d, want 1", len(parsed.Data))
	}
	if parsed.Data[0].Name != wantName {
		t.Errorf("data[0].name = %q, want %q", parsed.Data[0].Name, wantName)
	}
}

// TestClient_ErrorBodyParsing asserts that an error envelope in the body is
// surfaced as a *MetaAPIError with the correct code, even when returned with
// an HTTP 200 status.
func TestClient_ErrorBodyParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Deliberately return HTTP 200 to exercise the "error with 200" path.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"error":{"code":190,"message":"token expired"}}`))
	}))
	defer srv.Close()

	c := NewClient("test-token")
	c.baseURL = srv.URL

	_, err := c.doRequest(context.Background(), http.MethodGet, "/me", nil)
	if err == nil {
		t.Fatal("doRequest returned nil error, want MetaAPIError")
	}

	var metaErr *MetaAPIError
	if !errors.As(err, &metaErr) {
		t.Fatalf("errors.As(err, *MetaAPIError) = false; err = %v", err)
	}
	if metaErr.Code != 190 {
		t.Errorf("metaErr.Code = %d, want 190", metaErr.Code)
	}
	if metaErr.Message != "token expired" {
		t.Errorf("metaErr.Message = %q, want %q", metaErr.Message, "token expired")
	}
}

// TestClient_ErrorBodyWith400 asserts error parsing also works when the error
// envelope is paired with a non-2xx status.
func TestClient_ErrorBodyWith400(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":17,"message":"rate limit","type":"OAuthException"}}`))
	}))
	defer srv.Close()

	c := NewClient("test-token")
	c.baseURL = srv.URL

	_, err := c.doRequest(context.Background(), http.MethodGet, "/act_123/insights", nil)
	var metaErr *MetaAPIError
	if !errors.As(err, &metaErr) {
		t.Fatalf("errors.As(err, *MetaAPIError) = false; err = %v", err)
	}
	if metaErr.Code != 17 {
		t.Errorf("metaErr.Code = %d, want 17", metaErr.Code)
	}
	if metaErr.Type != "OAuthException" {
		t.Errorf("metaErr.Type = %q, want %q", metaErr.Type, "OAuthException")
	}
}

// TestClient_NonJSONErrorStatus asserts a non-2xx status with a non-JSON body
// still produces a MetaAPIError carrying the HTTP status code.
func TestClient_NonJSONErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal server error"))
	}))
	defer srv.Close()

	c := NewClient("test-token")
	c.baseURL = srv.URL

	_, err := c.doRequest(context.Background(), http.MethodGet, "/me", nil)
	var metaErr *MetaAPIError
	if !errors.As(err, &metaErr) {
		t.Fatalf("errors.As(err, *MetaAPIError) = false; err = %v", err)
	}
	if metaErr.Code != http.StatusInternalServerError {
		t.Errorf("metaErr.Code = %d, want %d", metaErr.Code, http.StatusInternalServerError)
	}
}
