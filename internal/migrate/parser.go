package migrate

import (
	"context"
	"io"

	"github.com/caioricciuti/etiquetta/internal/buffer"
)

// Source type constants
const (
	SourceGA4BigQuery = "ga4_bigquery"
	SourceGA4CSV      = "ga4_csv"
	SourcePlausible   = "plausible"
	SourceMatomo      = "matomo"
	SourceUmami       = "umami"
	SourceCSV         = "csv"
	SourceGTM         = "gtm"
)

// Detection holds the result of auto-detecting a file's format.
type Detection struct {
	Source           string            `json:"source"`
	Columns          []string          `json:"columns"`
	SampleRows       [][]string        `json:"sample_rows"`
	RowEstimate      int64             `json:"row_estimate"`
	DateRange        *[2]string        `json:"date_range"`
	SuggestedMapping map[string]string `json:"suggested_mapping"`
}

// ParseOpts holds configuration for parsing.
type ParseOpts struct {
	Domain   string
	ImportID string
	Mapping  map[string]string
	Timezone string
}

// Parser is the interface all source parsers implement.
type Parser interface {
	Detect(reader io.ReadSeeker) (*Detection, error)
	Parse(ctx context.Context, reader io.Reader, opts ParseOpts, emit func([]buffer.Event) error) error
}

// GetParser returns a Parser for the given source type. Returns nil if unknown.
func GetParser(source string) Parser {
	switch source {
	case SourcePlausible:
		return &PlausibleParser{}
	case SourceGA4BigQuery:
		return &GA4BigQueryParser{}
	case SourceGA4CSV:
		return &GA4CSVParser{}
	case SourceMatomo:
		return &MatomoParser{}
	case SourceUmami:
		return &UmamiParser{}
	case SourceCSV:
		return &GenericCSVParser{}
	default:
		return nil
	}
}
