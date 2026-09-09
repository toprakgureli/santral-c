// Package verimor integrates the Bulutsantralim (Verimor) hosted PBX: it mints
// webphone tokens, reads call records and places click-to-call originations.
package verimor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to the Bulutsantralim REST API.
type Client struct {
	apiKey string
	base   string
	http   *http.Client
}

// NewClient builds a Bulutsantralim API client.
func NewClient(apiKey, base string) *Client {
	if base == "" {
		base = "https://api.bulutsantralim.com"
	}
	return &Client{
		apiKey: apiKey,
		base:   strings.TrimRight(base, "/"),
		http:   &http.Client{Timeout: 15 * time.Second},
	}
}

// CDR is one call record as returned by the API.
type CDR struct {
	CallUUID          string `json:"call_uuid"`
	StartStamp        string `json:"start_stamp"`
	Direction         string `json:"direction"`
	CallerIDNumber    string `json:"caller_id_number"`
	DestinationNumber string `json:"destination_number"`
	Duration          string `json:"duration"`
	Result            string `json:"result"`
	Missed            bool   `json:"missed"`
	RecordingPresent  bool   `json:"recording_present"`
}

// Pagination is the paging envelope the CDR list returns.
type Pagination struct {
	Page       int `json:"page"`
	TotalCount int `json:"total_count"`
	TotalPages int `json:"total_pages"`
	Limit      int `json:"limit"`
}

type cdrList struct {
	CDRs       []CDR      `json:"cdrs"`
	Pagination Pagination `json:"pagination"`
}

// WebphoneToken mints a one-day webphone token for an extension.
func (c *Client) WebphoneToken(ctx context.Context, extension string) (string, error) {
	body, _ := json.Marshal(map[string]string{"key": c.apiKey, "extension": extension})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/webphone_tokens", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	raw, status, err := c.do(req)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("webphone token rejected (%d): %s", status, strings.TrimSpace(string(raw)))
	}

	var parsed struct {
		Token string `json:"token"`
	}
	if json.Unmarshal(raw, &parsed) == nil && parsed.Token != "" {
		return parsed.Token, nil
	}
	return strings.TrimSpace(string(raw)), nil
}

// CDRs lists call records filtered by params (key is added automatically).
func (c *Client) CDRs(ctx context.Context, params url.Values) ([]CDR, Pagination, error) {
	params.Set("key", c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/cdrs?"+params.Encode(), nil)
	if err != nil {
		return nil, Pagination{}, err
	}
	raw, status, err := c.do(req)
	if err != nil {
		return nil, Pagination{}, err
	}
	if status != http.StatusOK {
		return nil, Pagination{}, fmt.Errorf("cdr list failed (%d): %s", status, strings.TrimSpace(string(raw)))
	}
	var list cdrList
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, Pagination{}, fmt.Errorf("cdr response could not be parsed: %w", err)
	}
	return list.CDRs, list.Pagination, nil
}

// Originate places a click-to-call from extension to destination and returns
// the call UUID.
func (c *Client) Originate(ctx context.Context, extension, destination string) (string, error) {
	params := url.Values{}
	params.Set("key", c.apiKey)
	params.Set("extension", extension)
	params.Set("destination", destination)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/originate?"+params.Encode(), nil)
	if err != nil {
		return "", err
	}
	raw, status, err := c.do(req)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("originate rejected (%d): %s", status, strings.TrimSpace(string(raw)))
	}
	return strings.TrimSpace(string(raw)), nil
}

func (c *Client) do(req *http.Request) ([]byte, int, error) {
	res, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("bulutsantralim request failed: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, res.StatusCode, fmt.Errorf("bulutsantralim response could not be read: %w", err)
	}
	return raw, res.StatusCode, nil
}
