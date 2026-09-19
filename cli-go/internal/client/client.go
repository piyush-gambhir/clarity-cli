package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const ExportURL = "https://www.clarity.ms/export-data/api/v1/project-live-insights"
const MCPBaseURL = "https://clarity.microsoft.com/mcp"
const maxResponse = 32 << 20

type Client struct {
	Token string
	HTTP  *http.Client
	Log   io.Writer
}

type APIError struct {
	Status     int    `json:"status" yaml:"status"`
	Message    string `json:"message" yaml:"message"`
	RetryAfter string `json:"retry_after,omitempty" yaml:"retry_after,omitempty"`
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("Clarity: %s (HTTP %d)", e.Message, e.Status)
	if e.RetryAfter != "" {
		msg += "; Retry-After: " + e.RetryAfter
	}
	return msg
}

func New(token string, timeout time.Duration) *Client {
	return &Client{Token: token, HTTP: &http.Client{Timeout: timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		// Never forward project credentials to a redirect target.
		return http.ErrUseLastResponse
	}}}
}

func (c *Client) request(ctx context.Context, method, endpoint string, body any) (any, error) {
	if strings.TrimSpace(c.Token) == "" {
		return nil, fmt.Errorf("a Clarity API token is required")
	}
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "clarity-cli")
	if c.Log != nil {
		fmt.Fprintf(c.Log, "%s %s\n", method, req.URL.Redacted())
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %s", strings.ReplaceAll(err.Error(), c.Token, "[REDACTED]"))
	}
	defer res.Body.Close()
	if c.Log != nil {
		fmt.Fprintf(c.Log, "HTTP %d\n", res.StatusCode)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		// Avoid reflecting response bodies that may contain credentials or HTML.
		message := http.StatusText(res.StatusCode)
		switch res.StatusCode {
		case 401:
			message = "invalid, expired, or revoked token; generate a new project token and run clarity auth login"
		case 403:
			message = "token is not authorized for this operation"
		case 429:
			message = "rate limit exceeded; no automatic retry was attempted"
		}
		return nil, &APIError{Status: res.StatusCode, Message: message, RetryAfter: res.Header.Get("Retry-After")}
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, maxResponse+1))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if len(b) > maxResponse {
		return nil, fmt.Errorf("response exceeds 32 MiB")
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var data any
	if err := dec.Decode(&data); err != nil {
		return nil, fmt.Errorf("Clarity returned invalid JSON")
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("Clarity returned trailing content after JSON")
	}
	if data == nil {
		return nil, fmt.Errorf("Clarity returned null instead of data")
	}
	if m, ok := data.(map[string]any); ok {
		if m["isError"] == true || m["success"] == false || nonzeroError(m["dataErrorType"]) || nonemptyError(m["error"]) {
			return nil, fmt.Errorf("Clarity reported an application error despite HTTP success; check the query, dates, and project access")
		}
	}
	return data, nil
}

func nonzeroError(v any) bool {
	if v == nil {
		return false
	}
	s := fmt.Sprint(v)
	return s != "0" && s != "" && s != "None"
}

func nonemptyError(v any) bool { return v != nil && v != "" && v != false }

var Dimensions = []string{"Browser", "Device", "Country/Region", "OS", "Source", "Medium", "Campaign", "Channel", "URL"}

func ExportParams(days int, dimensions []string) (url.Values, error) {
	if days < 1 || days > 3 {
		return nil, fmt.Errorf("--days must be 1, 2, or 3 for the Export API")
	}
	if len(dimensions) > 3 {
		return nil, fmt.Errorf("the Export API accepts at most three dimensions")
	}
	q := url.Values{"numOfDays": {strconv.Itoa(days)}}
	seen := map[string]bool{}
	for i, dimension := range dimensions {
		canonical := ""
		for _, valid := range Dimensions {
			if strings.EqualFold(dimension, valid) {
				canonical = valid
				break
			}
		}
		if canonical == "" {
			return nil, fmt.Errorf("unsupported dimension %q; use %s", dimension, strings.Join(Dimensions, ", "))
		}
		if seen[canonical] {
			return nil, fmt.Errorf("duplicate dimension %q", canonical)
		}
		seen[canonical] = true
		q.Set(fmt.Sprintf("dimension%d", i+1), canonical)
	}
	return q, nil
}

func (c *Client) Export(ctx context.Context, days int, dimensions []string) (any, error) {
	q, err := ExportParams(days, dimensions)
	if err != nil {
		return nil, err
	}
	return c.request(ctx, http.MethodGet, ExportURL+"?"+q.Encode(), nil)
}

func (c *Client) Query(ctx context.Context, query, timezone string) (any, error) {
	return c.request(ctx, http.MethodPost, MCPBaseURL+"/dashboard/query", map[string]any{"query": query, "timezone": timezone})
}

func (c *Client) Documentation(ctx context.Context, query string) (any, error) {
	return c.request(ctx, http.MethodPost, MCPBaseURL+"/documentation/query", map[string]any{"query": query})
}

func (c *Client) Recordings(ctx context.Context, body map[string]any) (any, error) {
	return c.request(ctx, http.MethodPost, MCPBaseURL+"/recordings/sample", body)
}
