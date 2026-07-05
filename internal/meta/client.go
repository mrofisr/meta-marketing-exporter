// Package meta provides a client for the Meta (Facebook) Marketing API.
package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultBaseURL is the Meta Graph API base endpoint targeted by this client.
const DefaultBaseURL = "https://graph.facebook.com/v21.0"

// MetaAPIError is a typed representation of an error returned by the Meta
// Marketing API. Meta reports errors as a JSON object under the "error" key,
// sometimes even with an HTTP 200 status code, so this type captures the
// meaningful code/message/type for downstream handling (e.g. rate-limit or
// auth-expiry detection).
type MetaAPIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type"`
}

// Error implements the error interface so MetaAPIError can be returned as an
// error and matched via errors.As.
func (e *MetaAPIError) Error() string {
	if e.Type != "" {
		return fmt.Sprintf("meta api error (code %d, type %s): %s", e.Code, e.Type, e.Message)
	}
	return fmt.Sprintf("meta api error (code %d): %s", e.Code, e.Message)
}

// Client is a thin wrapper around net/http.Client that authenticates requests
// against the Meta Marketing API and parses typed errors from responses.
type Client struct {
	httpClient *http.Client
	baseURL    string
	token      string
}

// NewClient constructs a Client for the given access token using the default
// base URL and a sensible default HTTP timeout.
func NewClient(token string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    DefaultBaseURL,
		token:      token,
	}
}

// errorEnvelope models the shape Meta uses to report API errors:
// {"error": {"code": N, "message": "...", "type": "..."}}.
type errorEnvelope struct {
	Error *MetaAPIError `json:"error"`
}

// doRequest executes an authenticated request against the Meta API. The
// access_token param is appended automatically. path may be absolute
// ("/act_123/insights") or relative; it is joined onto the client base URL.
//
// On a non-2xx status OR a JSON body containing {"error": {...}} (Meta
// sometimes returns errors with HTTP 200), the error object is parsed into a
// *MetaAPIError and returned as the error. The raw body is returned only on
// success.
func (c *Client) doRequest(ctx context.Context, method, path string, params url.Values) ([]byte, error) {
	if params == nil {
		params = url.Values{}
	}
	params.Set("access_token", c.token)

	u := c.baseURL + "/" + strings.TrimLeft(path, "/")
	if encoded := params.Encode(); encoded != "" {
		u += "?" + encoded
	}

	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return nil, fmt.Errorf("meta: build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("meta: execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("meta: read response body: %w", err)
	}

	// Meta can embed an error object even with a 200 status, so always attempt
	// to detect the error envelope before treating the body as success.
	if apiErr := parseAPIError(body); apiErr != nil {
		return nil, apiErr
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &MetaAPIError{
			Code:    resp.StatusCode,
			Message: fmt.Sprintf("unexpected HTTP status %d: %s", resp.StatusCode, strings.TrimSpace(string(body))),
		}
	}

	return body, nil
}

// parseAPIError inspects a response body for a Meta error envelope and returns
// a *MetaAPIError if one is present, or nil otherwise. Non-JSON or
// error-free bodies yield nil.
func parseAPIError(body []byte) *MetaAPIError {
	if len(body) == 0 {
		return nil
	}
	var env errorEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil
	}
	if env.Error == nil {
		return nil
	}
	return env.Error
}
