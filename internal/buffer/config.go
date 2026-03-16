package buffer

import (
	"os"
	"strconv"
	"time"
)

// BufferConfig holds configuration for the write buffer layer.
type BufferConfig struct {
	// Threshold is the number of rows that triggers a flush.
	Threshold int
	// FlushTimeout is the max time between flushes.
	FlushTimeout time.Duration
	// TempDir is where parquet files are written before DuckDB reads them.
	TempDir string
	// CompactHour is the hour of day (0-23) to run compaction.
	CompactHour int
	// DuckDBPath is the path to the DuckDB database file.
	DuckDBPath string
}

// DefaultConfig returns a BufferConfig with sensible defaults, overridden by env vars.
func DefaultConfig(duckDBPath, dataDir string) BufferConfig {
	cfg := BufferConfig{
		Threshold:    50000,
		FlushTimeout: 30 * time.Second,
		TempDir:      dataDir + "/buffer_tmp",
		CompactHour:  3,
		DuckDBPath:   duckDBPath,
	}

	if v := os.Getenv("ETIQUETTA_BUFFER_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Threshold = n
		}
	}
	if v := os.Getenv("ETIQUETTA_BUFFER_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.FlushTimeout = d
		}
	}
	if v := os.Getenv("ETIQUETTA_BUFFER_TEMP_DIR"); v != "" {
		cfg.TempDir = v
	}

	return cfg
}
