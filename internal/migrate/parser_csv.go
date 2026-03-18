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

// GenericCSVParser is the fallback parser for arbitrary CSV files.
// It relies on user-provided Mapping to extract fields.
type GenericCSVParser struct{}

func (p *GenericCSVParser) Detect(reader io.ReadSeeker) (*Detection, error) {
	reader.Seek(0, io.SeekStart)
	cr := csv.NewReader(reader)

	headers, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("not a valid CSV: %w", err)
	}

	det := &Detection{
		Source:           SourceCSV,
		Columns:          headers,
		SuggestedMapping: map[string]string{},
	}

	for i := 0; i < 50; i++ {
		row, err := cr.Read()
		if err != nil {
			break
		}
		det.SampleRows = append(det.SampleRows, row)
		det.RowEstimate++
	}

	return det, nil
}

func (p *GenericCSVParser) Parse(ctx context.Context, reader io.Reader, opts ParseOpts, emit func([]buffer.Event) error) error {
	cr := csv.NewReader(reader)

	headers, err := cr.Read()
	if err != nil {
		return fmt.Errorf("reading headers: %w", err)
	}

	colIdx := indexColumns(headers)

	// Build reverse mapping: target field → source column (lowercased)
	reverseMap := make(map[string]string, len(opts.Mapping))
	for srcCol, target := range opts.Mapping {
		reverseMap[target] = strings.ToLower(srcCol)
	}

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

		// Extract mapped values
		getMapped := func(target string) string {
			srcCol, ok := reverseMap[target]
			if !ok {
				return ""
			}
			return getCol(row, colIdx, srcCol)
		}

		path := getMapped("path")
		if path != "" && !strings.HasPrefix(path, "/") {
			path = "/" + path
		}

		dateStr := getMapped("timestamp")
		baseTime, err := parseFlexibleDate(dateStr)
		if err != nil {
			// If no parseable date, skip row
			continue
		}

		countStr := getMapped("count")
		count, _ := strconv.Atoi(countStr)
		if count <= 0 {
			count = 1
		}

		events := generateSyntheticEvents(opts, baseTime, path, count)

		// Apply extra mapped fields to all generated events
		country := getMapped("country")
		browserName := getMapped("browser")
		osName := getMapped("os")
		deviceType := getMapped("device")
		referrer := getMapped("referrer")
		utmSource := getMapped("utm_source")
		utmMedium := getMapped("utm_medium")
		utmCampaign := getMapped("utm_campaign")
		eventName := getMapped("event_name")
		pageTitle := getMapped("page_title")

		for j := range events {
			if country != "" {
				events[j].GeoCountry = country
			}
			if browserName != "" {
				events[j].BrowserName = browserName
			}
			if osName != "" {
				events[j].OSName = osName
			}
			if deviceType != "" {
				events[j].DeviceType = strings.ToLower(deviceType)
			}
			if referrer != "" {
				events[j].ReferrerURL = referrer
			}
			if utmSource != "" {
				events[j].UTMSource = utmSource
			}
			if utmMedium != "" {
				events[j].UTMMedium = utmMedium
			}
			if utmCampaign != "" {
				events[j].UTMCampaign = utmCampaign
			}
			if eventName != "" {
				events[j].EventName = eventName
			}
			if pageTitle != "" {
				events[j].PageTitle = pageTitle
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
