package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/caioricciuti/etiquetta/internal/buffer"
)

// PlausibleParser handles Plausible CSV exports.
type PlausibleParser struct{}

func (p *PlausibleParser) Detect(reader io.ReadSeeker) (*Detection, error) {
	reader.Seek(0, io.SeekStart)
	cr := csv.NewReader(reader)

	headers, err := cr.Read()
	if err != nil {
		return nil, err
	}

	headerSet := make(map[string]bool, len(headers))
	for _, h := range headers {
		headerSet[strings.TrimSpace(strings.ToLower(h))] = true
	}

	if !headerSet["date"] || !headerSet["page"] || !headerSet["visitors"] || !headerSet["pageviews"] {
		return nil, fmt.Errorf("not a Plausible CSV: missing required columns")
	}

	det := &Detection{
		Source:  SourcePlausible,
		Columns: headers,
		SuggestedMapping: map[string]string{
			"date":      "timestamp",
			"page":      "path",
			"pageviews": "count",
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
		if len(row) > 0 {
			d := row[0]
			if minDate == "" || d < minDate {
				minDate = d
			}
			if maxDate == "" || d > maxDate {
				maxDate = d
			}
		}
	}

	if minDate != "" {
		det.DateRange = &[2]string{minDate, maxDate}
	}

	// Estimate total rows from file size
	reader.Seek(0, io.SeekEnd)

	return det, nil
}

func (p *PlausibleParser) Parse(ctx context.Context, reader io.Reader, opts ParseOpts, emit func([]buffer.Event) error) error {
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
		page := getCol(row, colIdx, "page")
		pvStr := getCol(row, colIdx, "pageviews")

		baseTime, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}

		count, err := strconv.Atoi(pvStr)
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

// --- shared helpers ---

// indexColumns maps lowercased column names to their indices.
func indexColumns(headers []string) map[string]int {
	m := make(map[string]int, len(headers))
	for i, h := range headers {
		m[strings.TrimSpace(strings.ToLower(h))] = i
	}
	return m
}

// getCol returns the value at the named column, or "" if missing.
func getCol(row []string, idx map[string]int, name string) string {
	i, ok := idx[strings.ToLower(name)]
	if !ok || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

// visitorHash returns a deterministic 16-char hex hash for a synthetic visitor.
func visitorHash(importID, date, path string, i int) string {
	h := sha256.Sum256([]byte(importID + date + path + strconv.Itoa(i)))
	return hex.EncodeToString(h[:])[:16]
}

// syntheticEventID returns a UUID-like ID for synthetic events.
func syntheticEventID(importID, date string, i int) string {
	prefix := importID
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	return fmt.Sprintf("%s-%s-%d", prefix, date, i)
}

// generateSyntheticEvents creates count synthetic pageview events spread across the day.
func generateSyntheticEvents(opts ParseOpts, baseTime time.Time, path string, count int) []buffer.Event {
	if count <= 0 {
		count = 1
	}

	dateStr := baseTime.Format("2006-01-02")
	intervalMs := int64(0)
	if count > 1 {
		intervalMs = (24 * 60 * 60 * 1000) / int64(count)
	}

	url := "https://" + opts.Domain + path

	events := make([]buffer.Event, 0, count)
	for i := 0; i < count; i++ {
		ts := baseTime.UnixMilli() + int64(i)*intervalMs
		vh := visitorHash(opts.ImportID, dateStr, path, i)

		events = append(events, buffer.Event{
			ID:          syntheticEventID(opts.ImportID, dateStr, i),
			Timestamp:   ts,
			EventType:   "pageview",
			EventName:   "",
			SessionID:   vh,
			VisitorHash: vh,
			Domain:      opts.Domain,
			URL:         url,
			Path:        path,
			Props:       "{}",
			BotScore:    0,
			BotCategory: "human",
			BotSignals:  "[]",
			IsBot:       0,
			ImportID:    opts.ImportID,
		})
	}
	return events
}
