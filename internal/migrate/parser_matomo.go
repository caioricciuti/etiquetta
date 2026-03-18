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

// MatomoParser handles Matomo CSV exports.
type MatomoParser struct{}

func (p *MatomoParser) Detect(reader io.ReadSeeker) (*Detection, error) {
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

	hasLabel := headerSet["label"]
	hasVisits := headerSet["nb_visits"] || headerSet["nb_actions"]

	if !hasLabel || !hasVisits {
		return nil, fmt.Errorf("not a Matomo CSV: missing required columns (label, nb_visits/nb_actions)")
	}

	det := &Detection{
		Source:  SourceMatomo,
		Columns: headers,
		SuggestedMapping: map[string]string{
			"date":       "timestamp",
			"label":      "path",
			"nb_actions": "count",
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

func (p *MatomoParser) Parse(ctx context.Context, reader io.Reader, opts ParseOpts, emit func([]buffer.Event) error) error {
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
		label := getCol(row, colIdx, "label")
		countStr := getCol(row, colIdx, "nb_actions")
		if countStr == "" {
			countStr = getCol(row, colIdx, "nb_visits")
		}

		baseTime, err := parseFlexibleDate(dateStr)
		if err != nil {
			continue
		}

		// Ensure label starts with /
		if label != "" && !strings.HasPrefix(label, "/") {
			label = "/" + label
		}

		count, err := strconv.Atoi(countStr)
		if err != nil || count <= 0 {
			count = 1
		}

		events := generateSyntheticEvents(opts, baseTime, label, count)
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
