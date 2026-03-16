package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// QueryResult represents the result of a SQL query
type QueryResult struct {
	Columns    []string        `json:"columns"`
	Rows       [][]interface{} `json:"rows"`
	RowCount   int             `json:"row_count"`
	DurationMs int64           `json:"duration_ms"`
}

// MaxQueryRows is the maximum number of rows returned
const MaxQueryRows = 1000

// QueryTimeout is the maximum query execution time
const QueryTimeout = 5 * time.Second

// dangerousKeywords are SQL keywords that modify data
var dangerousKeywords = []string{
	"INSERT", "UPDATE", "DELETE", "DROP", "ALTER", "CREATE",
	"TRUNCATE", "GRANT", "REVOKE", "ATTACH", "DETACH",
	"VACUUM", "COPY", "EXPORT", "IMPORT", "INSTALL", "LOAD",
}

// isReadOnlyQuery checks if a query is safe to execute
func isReadOnlyQuery(query string) bool {
	// Normalize: uppercase, remove comments, collapse whitespace
	normalized := strings.ToUpper(strings.TrimSpace(query))

	// Remove SQL comments
	singleLineComment := regexp.MustCompile(`--.*`)
	multiLineComment := regexp.MustCompile(`/\*[\s\S]*?\*/`)
	normalized = singleLineComment.ReplaceAllString(normalized, "")
	normalized = multiLineComment.ReplaceAllString(normalized, "")
	normalized = strings.TrimSpace(normalized)

	// Must start with SELECT or WITH (for CTEs)
	if !strings.HasPrefix(normalized, "SELECT") && !strings.HasPrefix(normalized, "WITH") {
		return false
	}

	// Check for dangerous keywords
	for _, kw := range dangerousKeywords {
		// Use word boundary check to avoid false positives
		pattern := regexp.MustCompile(`\b` + kw + `\b`)
		if pattern.MatchString(normalized) {
			return false
		}
	}

	return true
}

// ExecuteExplorerQuery executes a read-only SQL query with safety checks
func (db *DB) ExecuteExplorerQuery(query string) (*QueryResult, error) {
	// Validate query is read-only
	if !isReadOnlyQuery(query) {
		return nil, errors.New("only SELECT queries are allowed")
	}

	// Add LIMIT if not present to prevent huge result sets
	upperQuery := strings.ToUpper(query)
	if !strings.Contains(upperQuery, "LIMIT") {
		query = strings.TrimSuffix(strings.TrimSpace(query), ";")
		query = fmt.Sprintf("%s LIMIT %d", query, MaxQueryRows)
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), QueryTimeout)
	defer cancel()

	start := time.Now()

	// Execute query
	rows, err := db.conn.QueryContext(ctx, query)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, errors.New("query timeout exceeded (5 seconds max)")
		}
		return nil, err
	}
	defer rows.Close()

	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	// Scan rows
	resultRows := make([][]interface{}, 0)
	rowCount := 0

	for rows.Next() {
		if rowCount >= MaxQueryRows {
			break
		}

		// Create slice of interface{} to hold values
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}

		// Convert sql.RawBytes to string for JSON serialization
		row := make([]interface{}, len(columns))
		for i, v := range values {
			switch val := v.(type) {
			case []byte:
				row[i] = string(val)
			case sql.RawBytes:
				row[i] = string(val)
			default:
				row[i] = val
			}
		}

		resultRows = append(resultRows, row)
		rowCount++
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	duration := time.Since(start).Milliseconds()

	return &QueryResult{
		Columns:    columns,
		Rows:       resultRows,
		RowCount:   rowCount,
		DurationMs: duration,
	}, nil
}

// AllowedExplorerTables are the tables accessible in the Data Explorer
var AllowedExplorerTables = map[string]bool{
	"campaigns":        true,
	"domains":          true,
	"errors":           true,
	"events":           true,
	"performance":      true,
	"visitor_sessions": true,
}

// GetTableSchema returns the schema for allowed tables only
func (db *DB) GetTableSchema() (map[string][]map[string]string, error) {
	schema := make(map[string][]map[string]string)

	// Only return schema for allowed tables
	for table := range AllowedExplorerTables {
		rows, err := db.conn.Query(`
			SELECT column_name, data_type
			FROM information_schema.columns
			WHERE table_name = ?
			ORDER BY ordinal_position
		`, table)
		if err != nil {
			continue
		}

		var columns []map[string]string
		for rows.Next() {
			var name, colType string
			if err := rows.Scan(&name, &colType); err != nil {
				continue
			}

			columns = append(columns, map[string]string{
				"name": name,
				"type": colType,
			})
		}
		rows.Close()

		if len(columns) > 0 {
			schema[table] = columns
		}
	}

	return schema, nil
}
