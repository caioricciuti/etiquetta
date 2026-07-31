package main

import (
	"bufio"
	"os"
	"strings"
)

// loadDotEnv reads a .env file and populates the process environment for any
// key that is not already set. The real environment always wins, so an inline
// override like `ETIQUETTA_STORAGE=duckdb etiquetta serve` still beats the file.
//
// It must run before anything reads configuration: the flag defaults in main's
// init() and every os.Getenv call downstream. It is therefore called as the
// first statement of init(). Missing file is not an error — running without a
// .env is the normal case.
//
// Supported syntax (a deliberately small subset, no dependency):
//   - KEY=value
//   - export KEY=value          (leading `export ` is ignored)
//   - # full-line comments and blank lines are skipped
//   - KEY="value" / KEY='value' (matching surrounding quotes are stripped)
//
// Inline comments after an unquoted value are NOT stripped, so a value may
// safely contain a `#` (e.g. a URL fragment or a generated secret).
func loadDotEnv() {
	path := os.Getenv("ETIQUETTA_ENV_FILE")
	if path == "" {
		path = ".env"
	}
	f, err := os.Open(path)
	if err != nil {
		return // no .env (or unreadable) — nothing to load
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue // no key, or "=value" with an empty key
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		if key == "" {
			continue
		}
		// Strip one layer of matching surrounding quotes.
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') ||
				(val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
}
