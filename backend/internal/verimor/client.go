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
	apiKey   string
	base     string
	http     *http.Client
	download *http.Client // longer timeout for streaming recordings
}

// NewClient builds a Bulutsantralim API client.
func NewClient(apiKey, base string) *Client {
	if base == "" {
		base = "https://api.bulutsantralim.com"
	}
	return &Client{
		apiKey:   apiKey,
		base:     strings.TrimRight(base, "/"),
		http:     &http.Client{Timeout: 15 * time.Second},
		download: &http.Client{Timeout: 60 * time.Second},
	}
}

// CDR is one call record as returned by the API.
type CDR struct {
	CallUUID          string   `json:"call_uuid"`
	StartStamp        string   `json:"start_stamp"`
	Direction         string   `json:"direction"`
	CallerIDNumber    string   `json:"caller_id_number"`
	DestinationNumber string   `json:"destination_number"`
	Duration          string   `json:"duration"`
	TalkDuration      string   `json:"talk_duration"`
	AnswerStamp       string   `json:"answer_stamp"`
	Result            string   `json:"result"`
	Missed            flexBool `json:"missed"`
	RecordingPresent  flexBool `json:"recording_present"`
}

// flexBool decodes a boolean that the hosted API may send either as a real JSON
// bool (true) or as a quoted string ("true"/"false"). A single string-typed
// field would otherwise fail the whole CDR list decode.
type flexBool bool

// UnmarshalJSON accepts true/false, "true"/"false", 1/0 and null.
func (b *flexBool) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"`)
	*b = s == "true" || s == "1"
	return nil
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

// Extension is one extension and its live status.
type Extension struct {
	User   int    `json:"user"`
	Status string `json:"status"`
}

// Queue is one call queue.
type Queue struct {
	Number int    `json:"number"`
	Name   string `json:"name"`
}

// UserStatuses lists extensions and their live status.
func (c *Client) UserStatuses(ctx context.Context) ([]Extension, error) {
	params := url.Values{}
	params.Set("key", c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/user_statuses?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	raw, status, err := c.do(req)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("user statuses failed (%d): %s", status, strings.TrimSpace(string(raw)))
	}
	var out []Extension
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("user statuses could not be parsed: %w", err)
	}
	return out, nil
}

// Queues lists the call queues.
func (c *Client) Queues(ctx context.Context) ([]Queue, error) {
	params := url.Values{}
	params.Set("key", c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/queues?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	raw, status, err := c.do(req)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("queues failed (%d): %s", status, strings.TrimSpace(string(raw)))
	}
	var out []Queue
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("queues could not be parsed: %w", err)
	}
	return out, nil
}

// SetDND turns do-not-disturb on or off for an extension (on = no calls).
func (c *Client) SetDND(ctx context.Context, extension string, on bool) error {
	state := "off"
	if on {
		state = "on"
	}
	params := url.Values{}
	params.Set("key", c.apiKey)
	params.Set("state", state)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/dnd/"+url.PathEscape(extension)+"?"+params.Encode(), nil)
	if err != nil {
		return err
	}
	raw, status, err := c.do(req)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("dnd change failed (%d): %s", status, strings.TrimSpace(string(raw)))
	}
	return nil
}

// CDRCount returns the total number of call records matching params.
func (c *Client) CDRCount(ctx context.Context, params url.Values) (int, error) {
	params.Set("limit", "10")
	_, pg, err := c.CDRs(ctx, params)
	if err != nil {
		return 0, err
	}
	return pg.TotalCount, nil
}

// RecordingURL mints a one-time download URL for a call's recording via
// POST /recording_url/ (key + call_uuid, form-encoded).
func (c *Client) RecordingURL(ctx context.Context, callUUID string) (string, error) {
	form := url.Values{}
	form.Set("key", c.apiKey)
	form.Set("call_uuid", callUUID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/recording_url/", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	raw, status, err := c.do(req)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("recording url failed (%d): %s", status, strings.TrimSpace(string(raw)))
	}
	u := strings.TrimSpace(strings.Trim(strings.TrimSpace(string(raw)), `"`))
	if u == "" {
		return "", fmt.Errorf("recording url empty")
	}
	return u, nil
}

// OpenRecording GETs a minted recording URL and returns the live response so the
// caller can stream it. The caller must close the body.
func (c *Client) OpenRecording(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.download.Do(req)
	if err != nil {
		return nil, fmt.Errorf("recording could not be fetched: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		_ = res.Body.Close()
		return nil, fmt.Errorf("recording download failed (%d)", res.StatusCode)
	}
	return res, nil
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
