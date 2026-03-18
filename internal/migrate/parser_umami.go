package migrate

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/caioricciuti/etiquetta/internal/buffer"
)

// UmamiParser handles Umami CSV exports.
type UmamiParser struct{}

func (p *UmamiParser) Detect(reader io.ReadSeeker) (*Detection, error) {
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

	if !headerSet["url"] || !headerSet["pageviews"] {
		return nil, fmt.Errorf("not an Umami CSV: missing required columns (url, pageviews)")
	}

	det := &Detection{
		Source:  SourceUmami,
		Columns: headers,
		SuggestedMapping: map[string]string{
			"date":      "timestamp",
			"url":       "path",
			"pageviews": "count",
			"country":   "country",
			"browser":   "browser",
			"os":        "os",
			"device":    "device",
			"referrer":  "referrer",
		},
	}

	colIdx := indexColumns(headers)
	var minDate, maxDate string
	for i := 0; i < 50; i++ {
		row, err := cr.Read()
		if err != nil {
			break
		}
		det.SampleRows = append(det.SampleRows, row)
		det.RowEstimate++

		d := getCol(row, colIdx, "date")
		if d != "" {
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

	return det, nil
}

func (p *UmamiParser) Parse(ctx context.Context, reader io.Reader, opts ParseOpts, emit func([]buffer.Event) error) error {
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
		urlPath := getCol(row, colIdx, "url")
		pvStr := getCol(row, colIdx, "pageviews")
		country := getCol(row, colIdx, "country")
		browserName := getCol(row, colIdx, "browser")
		osName := getCol(row, colIdx, "os")
		deviceType := getCol(row, colIdx, "device")
		referrer := getCol(row, colIdx, "referrer")

		baseTime, err := parseFlexibleDate(dateStr)
		if err != nil {
			continue
		}

		// Ensure path starts with /
		if urlPath != "" && !strings.HasPrefix(urlPath, "/") {
			urlPath = "/" + urlPath
		}

		count, err := strconv.Atoi(pvStr)
		if err != nil || count <= 0 {
			count = 1
		}

		// Generate synthetic events with extra Umami fields
		events := generateSyntheticEvents(opts, baseTime, urlPath, count)
		for j := range events {
			events[j].GeoCountry = country
			events[j].BrowserName = browserName
			events[j].OSName = osName
			events[j].DeviceType = strings.ToLower(deviceType)
			if referrer != "" {
				events[j].ReferrerURL = referrer
			}
		}

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
