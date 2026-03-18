package migrate

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/caioricciuti/etiquetta/internal/buffer"
)

// --- GA4 BigQuery (NDJSON) Parser ---

// GA4BigQueryParser handles GA4 BigQuery NDJSON exports (one event per line).
type GA4BigQueryParser struct{}

func (p *GA4BigQueryParser) Detect(reader io.ReadSeeker) (*Detection, error) {
	reader.Seek(0, io.SeekStart)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	if !scanner.Scan() {
		return nil, fmt.Errorf("empty file")
	}

	line := scanner.Text()
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		return nil, fmt.Errorf("not NDJSON: %w", err)
	}

	_, hasDate := obj["event_date"]
	_, hasTS := obj["event_timestamp"]
	if !hasDate && !hasTS {
		return nil, fmt.Errorf("not a GA4 BigQuery export: missing event_date/event_timestamp")
	}

	// Collect column names from first object
	columns := make([]string, 0, len(obj))
	for k := range obj {
		columns = append(columns, k)
	}

	det := &Detection{
		Source:  SourceGA4BigQuery,
		Columns: columns,
		SuggestedMapping: map[string]string{
			"event_timestamp": "timestamp",
			"page_location":   "path",
			"event_name":      "event_name",
		},
	}

	// Sample rows as string arrays
	det.SampleRows = append(det.SampleRows, []string{line})
	det.RowEstimate = 1

	for i := 0; i < 49 && scanner.Scan(); i++ {
		det.SampleRows = append(det.SampleRows, []string{scanner.Text()})
		det.RowEstimate++
	}

	return det, nil
}

func (p *GA4BigQueryParser) Parse(ctx context.Context, reader io.Reader, opts ParseOpts, emit func([]buffer.Event) error) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	batch := make([]buffer.Event, 0, 5000)

	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			continue
		}

		ev := ga4BJEventToBuffer(obj, opts)
		batch = append(batch, ev)

		if len(batch) >= 5000 {
			if err := emit(batch); err != nil {
				return err
			}
			batch = batch[:0]
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanning: %w", err)
	}

	if len(batch) > 0 {
		return emit(batch)
	}
	return nil
}

func ga4BJEventToBuffer(obj map[string]interface{}, opts ParseOpts) buffer.Event {
	ev := buffer.Event{
		Domain:      opts.Domain,
		Props:       "{}",
		BotScore:    0,
		BotCategory: "human",
		BotSignals:  "[]",
		IsBot:       0,
		ImportID:    opts.ImportID,
	}

	// Timestamp: event_timestamp is microseconds
	if tsRaw, ok := obj["event_timestamp"]; ok {
		switch v := tsRaw.(type) {
		case float64:
			ev.Timestamp = int64(v) / 1000 // microseconds to milliseconds
		case string:
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				ev.Timestamp = n / 1000
			}
		}
	}

	// Fallback to event_date if no timestamp
	if ev.Timestamp == 0 {
		if dateStr, ok := obj["event_date"].(string); ok {
			if t, err := time.Parse("20060102", dateStr); err == nil {
				ev.Timestamp = t.UnixMilli()
			}
		}
	}

	// Event name
	eventName, _ := obj["event_name"].(string)
	ev.EventName = eventName
	switch eventName {
	case "page_view":
		ev.EventType = "pageview"
	case "first_visit":
		ev.EventType = "pageview"
	default:
		ev.EventType = "custom"
	}

	// Extract page_location from event_params
	pageLocation := extractEventParam(obj, "page_location")
	if pageLocation != "" {
		ev.URL = pageLocation
		ev.Path = extractPath(pageLocation)
	}

	// Page title
	ev.PageTitle = extractEventParam(obj, "page_title")

	// Page referrer
	ev.ReferrerURL = extractEventParam(obj, "page_referrer")

	// User pseudo ID as visitor hash
	if uid, ok := obj["user_pseudo_id"].(string); ok {
		ev.VisitorHash = uid
	}

	// Session ID
	sessionID := extractEventParam(obj, "ga_session_id")
	if sessionID != "" {
		ev.SessionID = sessionID
	} else {
		ev.SessionID = ev.VisitorHash
	}

	// Generate an ID
	ev.ID = fmt.Sprintf("ga4-%d-%s", ev.Timestamp, ev.VisitorHash)
	if len(ev.ID) > 40 {
		ev.ID = ev.ID[:40]
	}

	// Geo
	if geo, ok := obj["geo"].(map[string]interface{}); ok {
		ev.GeoCountry, _ = geo["country"].(string)
		ev.GeoCity, _ = geo["city"].(string)
		ev.GeoRegion, _ = geo["region"].(string)
	}

	// Device
	if device, ok := obj["device"].(map[string]interface{}); ok {
		ev.BrowserName, _ = device["web_info"].(string)
		if cat, ok := device["category"].(string); ok {
			ev.DeviceType = cat
		}
		if os, ok := device["operating_system"].(string); ok {
			ev.OSName = os
		}
		if br, ok := device["browser"].(string); ok {
			ev.BrowserName = br
		}
	}

	// Traffic source
	if ts, ok := obj["traffic_source"].(map[string]interface{}); ok {
		ev.UTMSource, _ = ts["source"].(string)
		ev.UTMMedium, _ = ts["medium"].(string)
		ev.UTMCampaign, _ = ts["name"].(string)
	}

	return ev
}

// extractEventParam retrieves a string value from the GA4 event_params array.
func extractEventParam(obj map[string]interface{}, key string) string {
	params, ok := obj["event_params"].([]interface{})
	if !ok {
		return ""
	}
	for _, param := range params {
		pm, ok := param.(map[string]interface{})
		if !ok {
			continue
		}
		if pm["key"] != key {
			continue
		}
		if val, ok := pm["value"].(map[string]interface{}); ok {
			if sv, ok := val["string_value"].(string); ok {
				return sv
			}
			if iv, ok := val["int_value"].(float64); ok {
				return strconv.FormatInt(int64(iv), 10)
			}
		}
	}
	return ""
}

// extractPath pulls the path component from a URL string.
func extractPath(rawURL string) string {
	// Simple extraction: find path after host
	if idx := strings.Index(rawURL, "://"); idx >= 0 {
		rest := rawURL[idx+3:]
		if slash := strings.Index(rest, "/"); slash >= 0 {
			path := rest[slash:]
			if q := strings.Index(path, "?"); q >= 0 {
				path = path[:q]
			}
			if f := strings.Index(path, "#"); f >= 0 {
				path = path[:f]
			}
			return path
		}
		return "/"
	}
	return rawURL
}

// --- GA4 CSV (Aggregated Reports) Parser ---

// GA4CSVParser handles GA4 aggregated CSV report exports.
type GA4CSVParser struct{}

func (p *GA4CSVParser) Detect(reader io.ReadSeeker) (*Detection, error) {
	reader.Seek(0, io.SeekStart)
	cr := csv.NewReader(reader)

	headers, err := cr.Read()
	if err != nil {
		return nil, err
	}

	headerSet := make(map[string]bool, len(headers))
	lowerHeaders := make([]string, len(headers))
	for i, h := range headers {
		lower := strings.TrimSpace(strings.ToLower(h))
		headerSet[lower] = true
		lowerHeaders[i] = lower
	}

	hasDate := headerSet["date"]
	hasPage := headerSet["page path"] || headerSet["page"] || headerSet["page path and screen class"]
	hasViews := headerSet["views"] || headerSet["screenpage views"] || headerSet["pageviews"]

	if !hasDate || !hasPage || !hasViews {
		return nil, fmt.Errorf("not a GA4 CSV report: missing required columns")
	}

	det := &Detection{
		Source:  SourceGA4CSV,
		Columns: headers,
		SuggestedMapping: map[string]string{
			"date":  "timestamp",
			"page":  "path",
			"views": "count",
		},
	}

	var minDate, maxDate string
	for i := 0; i < 50; i++ {
		row, err := cr.Read()
		if err != nil {
			break
		}
		det.SampleRows = append(det.SampleRows, row)
		det.RowEstimate++

		dateVal := getCol(row, indexColumns(headers), "date")
		if dateVal != "" {
			if minDate == "" || dateVal < minDate {
				minDate = dateVal
			}
			if maxDate == "" || dateVal > maxDate {
				maxDate = dateVal
			}
		}
	}

	if minDate != "" {
		det.DateRange = &[2]string{minDate, maxDate}
	}

	return det, nil
}

func (p *GA4CSVParser) Parse(ctx context.Context, reader io.Reader, opts ParseOpts, emit func([]buffer.Event) error) error {
	cr := csv.NewReader(reader)

	headers, err := cr.Read()
	if err != nil {
		return fmt.Errorf("reading headers: %w", err)
	}

	colIdx := indexColumns(headers)

	batch := make([]buffer.Event, 0, 5000)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		row, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading row: %w", err)
		}

		dateStr := getCol(row, colIdx, "date")

		// Try path column names
		page := getCol(row, colIdx, "page path")
		if page == "" {
			page = getCol(row, colIdx, "page")
		}
		if page == "" {
			page = getCol(row, colIdx, "page path and screen class")
		}

		// Try views column names
		viewsStr := getCol(row, colIdx, "views")
		if viewsStr == "" {
			viewsStr = getCol(row, colIdx, "screenpage views")
		}
		if viewsStr == "" {
			viewsStr = getCol(row, colIdx, "pageviews")
		}

		// Parse date: GA4 uses YYYYMMDD
		baseTime, err := parseFlexibleDate(dateStr)
		if err != nil {
			continue
		}

		count, err := strconv.Atoi(viewsStr)
		if err != nil || count <= 0 {
			count = 1
		}

		events := generateSyntheticEvents(opts, baseTime, page, count)
		batch = append(batch, events...)

		if len(batch) >= 5000 {
			if err := emit(batch); err != nil {
				return err
			}
			batch = batch[:0]
		}
	}

	if len(batch) > 0 {
		return emit(batch)
	}
	return nil
}

// parseFlexibleDate tries multiple date formats.
func parseFlexibleDate(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02",
		"20060102",
		"2006-01-02 15:04:05",
		time.RFC3339,
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unable to parse date: %s", s)
}
