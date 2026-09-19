package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestEndpointContracts(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	body, err := RecordingBody(start, start.Add(24*time.Hour), 25, "SessionDuration_DESC", map[string]any{"rageClickPresent": true, "deviceType": []any{"Mobile"}})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, method, path string
		run                func(*Client) (any, error)
		check              func(*testing.T, *http.Request)
	}{
		{"export", "GET", "/export-data/api/v1/project-live-insights", func(c *Client) (any, error) {
			return c.Export(context.Background(), 3, []string{"device", "Country/Region"})
		}, func(t *testing.T, r *http.Request) {
			if r.URL.Host != "www.clarity.ms" || r.URL.Query().Get("numOfDays") != "3" || r.URL.Query().Get("dimension1") != "Device" || r.URL.Query().Get("dimension2") != "Country/Region" {
				t.Fatalf("bad export URL %s", r.URL)
			}
		}},
		{"query", "POST", "/mcp/dashboard/query", func(c *Client) (any, error) {
			return c.Query(context.Background(), "Traffic last week", "Asia/Kolkata")
		}, func(t *testing.T, r *http.Request) {
			var v map[string]any
			json.NewDecoder(r.Body).Decode(&v)
			if v["query"] != "Traffic last week" || v["timezone"] != "Asia/Kolkata" {
				t.Fatalf("bad body %#v", v)
			}
		}},
		{"docs", "POST", "/mcp/documentation/query", func(c *Client) (any, error) { return c.Documentation(context.Background(), "custom events") }, func(t *testing.T, r *http.Request) {
			var v map[string]any
			json.NewDecoder(r.Body).Decode(&v)
			if v["query"] != "custom events" || len(v) != 1 {
				t.Fatalf("bad body %#v", v)
			}
		}},
		{"recordings", "POST", "/mcp/recordings/sample", func(c *Client) (any, error) { return c.Recordings(context.Background(), body) }, func(t *testing.T, r *http.Request) {
			var v map[string]any
			json.NewDecoder(r.Body).Decode(&v)
			f := v["filters"].(map[string]any)
			date := f["date"].(map[string]any)
			if v["sortBy"] != float64(3) || v["count"] != float64(25) || v["start"] != "2026-09-01T00:00:00.000Z" || date["start"] != v["start"] || date["end"] != v["end"] || f["rageClickPresent"] != true {
				t.Fatalf("bad body %#v", v)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New("secret-token", time.Second)
			calls := 0
			c.HTTP.Transport = transport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != tt.method || r.URL.Path != tt.path || r.Header.Get("Authorization") != "Bearer secret-token" {
					t.Fatalf("wrong request %s %s", r.Method, r.URL)
				}
				tt.check(t, r)
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"count":9007199254740993}`)), Header: http.Header{}}, nil
			})
			v, err := tt.run(c)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("calls=%d", calls)
			}
			if v.(map[string]any)["count"].(json.Number).String() != "9007199254740993" {
				t.Fatal("lost precision")
			}
		})
	}
}

func TestErrorsNoRetryOrSecretLeak(t *testing.T) {
	for _, status := range []int{301, 400, 401, 403, 429, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			c := New("secret", time.Second)
			calls := 0
			c.HTTP.Transport = transport(func(r *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: status, Header: http.Header{"Retry-After": []string{"3600"}}, Body: io.NopCloser(strings.NewReader("secret"))}, nil
			})
			_, err := c.Export(context.Background(), 1, nil)
			var api *APIError
			if !errors.As(err, &api) || api.Status != status || api.RetryAfter != "3600" || calls != 1 || strings.Contains(err.Error(), "secret") {
				t.Fatalf("bad error: %v calls=%d", err, calls)
			}
		})
	}
	c := New("secret", time.Second)
	if c.HTTP.CheckRedirect(&http.Request{}, nil) != http.ErrUseLastResponse {
		t.Fatal("redirects enabled")
	}
	for _, body := range []string{`<html>oops</html>`, `null`, `{"dataErrorType":4}`, `{"isError":true}`, `{"error":{"message":"bad"}}`, `{"success":false}`, `{} {}`} {
		c.HTTP.Transport = transport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		if _, err := c.Export(context.Background(), 1, nil); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}

func TestValidation(t *testing.T) {
	for _, days := range []int{-1, 0, 4} {
		if _, err := ExportParams(days, nil); err == nil {
			t.Fatalf("accepted %d", days)
		}
	}
	for _, dims := range [][]string{{"made-up"}, {"OS", "os"}, {"OS", "URL", "Device", "Browser"}} {
		if _, err := ExportParams(1, dims); err == nil {
			t.Fatalf("accepted %v", dims)
		}
	}
	for _, f := range []map[string]any{
		{"unknown": true}, {"deviceType": []any{"Desktop"}}, {"rageClickPresent": "true"}, {"scrollDepth": map[string]any{"min": float64(101), "max": nil}},
		{"sessionDuration": map[string]any{"min": float64(5), "max": float64(1)}}, {"visitedUrls": []any{map[string]any{"url": "/checkout", "operator": "bad"}}},
	} {
		if err := ValidateFilters(f); err == nil {
			t.Fatalf("accepted %#v", f)
		}
	}
	now := time.Now()
	if _, err := RecordingBody(now, now.Add(time.Hour), 251, SortOptions[0], nil); err == nil {
		t.Fatal("accepted excess count")
	}
	if _, err := RecordingBody(now, now, 5, SortOptions[0], nil); err == nil {
		t.Fatal("accepted empty range")
	}
	if _, err := RecordingBody(now, now.Add(time.Hour), 5, "bad", nil); err == nil {
		t.Fatal("accepted invalid sort")
	}
}
