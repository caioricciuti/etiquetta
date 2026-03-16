package buffer

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/parquet-go/parquet-go"
)

// writeParquet writes rows to a parquet file in tempDir and returns the file path.
func writeParquet[T any](tableName string, rows []T, tempDir string) (string, error) {
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	fileName := fmt.Sprintf("%s_%d.parquet", tableName, time.Now().UnixNano())
	filePath := filepath.Join(tempDir, fileName)

	f, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("create parquet file: %w", err)
	}

	writer := parquet.NewGenericWriter[T](f)

	if _, err := writer.Write(rows); err != nil {
		f.Close()
		os.Remove(filePath)
		return "", fmt.Errorf("write parquet rows: %w", err)
	}

	if err := writer.Close(); err != nil {
		f.Close()
		os.Remove(filePath)
		return "", fmt.Errorf("close parquet writer: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(filePath)
		return "", fmt.Errorf("close parquet file: %w", err)
	}

	return filePath, nil
}
