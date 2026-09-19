package client

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

var SortOptions = []string{"SessionStart_DESC", "SessionStart_ASC", "SessionDuration_ASC", "SessionDuration_DESC", "SessionClickCount_ASC", "SessionClickCount_DESC", "PageCount_ASC", "PageCount_DESC"}

// Schema mirrors microsoft/clarity-mcp-server/src/types.ts, checked 2026-09-20.
var FilterKinds = func() map[string]string {
	m := map[string]string{"date": "date"}
	for kind, fields := range map[string]string{
		"string":  "referringUrl userType sessionIntent clickedText productName",
		"strings": "country city state deviceType browser os source medium campaign channel smartEvents javascriptErrors clickErrors productBrand checkoutAbandonmentStep",
		"bool":    "enteredTextPresent selectedTextPresent resizeEventPresent cursorMovement deadClickPresent rageClickPresent excessiveScrollPresent quickbackClickPresent productPurchases productAvailability",
		"range":   "visiblePageDuration hiddenPageDuration pageDuration sessionDuration pagesCount pageClickEventCount sessionClickEventCount largestContentfulPaint cumulativeLayoutShift firstInputDelay productRating productRatingsCount productPrice",
		"percent": "scrollDepth performanceScore",
		"urls":    "visitedUrls entryUrls exitUrls",
	} {
		for _, field := range strings.Fields(fields) {
			m[field] = kind
		}
	}
	return m
}()

var filterEnums = map[string][]string{
	"userType":      {"NewUser", "ReturningUser"},
	"sessionIntent": {"Low Intention", "Medium Intention", "High Intention"},
	"deviceType":    {"Mobile", "Tablet", "PC", "Email", "Other"},
	"browser":       {"Bot", "MiuiBrowser", "Chrome", "CoralWebView", "Edge", "Other", "Firefox", "IE", "Unknown", "Headless", "MobileApp", "Opera", "OperaMini", "Safari", "Samsung", "SamsungInternet", "Sogou", "UCBrowser", "YandexBrowser", "QQBrowser"},
	"os":            {"BlackBerry", "Android", "ChromeOS", "iOS", "Linux", "MacOS", "Other", "Windows", "WindowsMobile"},
	"channel":       {"OrganicSearch", "Direct", "Email", "Display", "Social", "PaidSearch", "Other", "Affiliate", "Referral", "Video", "Audio", "SMS", "AITools", "PaidAITools"},
}

func member(value string, choices []string) bool {
	for _, choice := range choices {
		if choice == value {
			return true
		}
	}
	return false
}

func ValidateFilters(filters map[string]any) error {
	keys := make([]string, 0, len(filters))
	for key := range filters {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := filters[key]
		kind, ok := FilterKinds[key]
		if !ok {
			return fmt.Errorf("unsupported filter %q; run clarity recordings filters", key)
		}
		valid := true
		switch kind {
		case "string":
			s, ok := value.(string)
			valid = ok
			if choices, exists := filterEnums[key]; exists && ok {
				valid = member(s, choices)
			}
		case "strings":
			values, ok := value.([]any)
			valid = ok
			for _, v := range values {
				s, ok := v.(string)
				if !ok {
					valid = false
					break
				}
				if choices, exists := filterEnums[key]; exists && !member(s, choices) {
					valid = false
					break
				}
			}
		case "bool":
			_, valid = value.(bool)
		case "range", "percent":
			m, ok := value.(map[string]any)
			valid = ok && len(m) == 2
			for _, bound := range []string{"min", "max"} {
				v, exists := m[bound]
				if !exists {
					valid = false
					continue
				}
				if v == nil {
					continue
				}
				n, ok := v.(float64)
				if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || (kind == "percent" && n > 100) {
					valid = false
				}
			}
			min, minOK := m["min"].(float64)
			max, maxOK := m["max"].(float64)
			if minOK && maxOK && min > max {
				valid = false
			}
		case "urls":
			values, ok := value.([]any)
			valid = ok
			for _, v := range values {
				m, ok := v.(map[string]any)
				if !ok || len(m) != 2 {
					valid = false
					break
				}
				u, ok := m["url"].(string)
				if !ok || u == "" {
					valid = false
				}
				op, ok := m["operator"].(string)
				if !ok || !member(op, []string{"contains", "startsWith", "endsWith", "excludes", "isExactly", "isExactlyNot", "matchesRegex", "excludesRegex"}) {
					valid = false
				}
			}
		case "date":
			m, ok := value.(map[string]any)
			valid = ok && len(m) == 2
			for _, key := range []string{"start", "end"} {
				s, ok := m[key].(string)
				if !ok {
					valid = false
					continue
				}
				if _, err := time.Parse(time.RFC3339Nano, s); err != nil {
					valid = false
				}
			}
		}
		if !valid {
			return fmt.Errorf("invalid %s filter %q; see clarity recordings filters", kind, key)
		}
	}
	return nil
}

func RecordingBody(start, end time.Time, count int, sortBy string, filters map[string]any) (map[string]any, error) {
	if !start.Before(end) {
		return nil, fmt.Errorf("start must be before end")
	}
	if count < 1 || count > 250 {
		return nil, fmt.Errorf("--count must be between 1 and 250")
	}
	sortID := -1
	for i, option := range SortOptions {
		if sortBy == option {
			sortID = i
			break
		}
	}
	if sortID < 0 {
		return nil, fmt.Errorf("invalid sort %q; use %s", sortBy, strings.Join(SortOptions, ", "))
	}
	copyFilters := map[string]any{}
	for k, v := range filters {
		copyFilters[k] = v
	}
	const iso = "2006-01-02T15:04:05.000Z"
	startString, endString := start.UTC().Format(iso), end.UTC().Format(iso)
	copyFilters["date"] = map[string]any{"start": startString, "end": endString}
	if err := ValidateFilters(copyFilters); err != nil {
		return nil, err
	}
	return map[string]any{"start": startString, "end": endString, "sortBy": sortID, "count": count, "filters": copyFilters}, nil
}

func FilterSchema() []map[string]any {
	keys := make([]string, 0, len(FilterKinds))
	for key := range FilterKinds {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	rows := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		row := map[string]any{"field": key, "type": FilterKinds[key]}
		if enum, ok := filterEnums[key]; ok {
			row["values"] = enum
		}
		rows = append(rows, row)
	}
	return rows
}
