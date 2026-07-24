package buffer

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/parquet-go/parquet-go"
)

// writeParquet writes rows to a parquet file in tempDir and returns the file path.
// The file is written under a .tmp name and renamed into place only once fully
// written, so a crash mid-write never leaves a truncated .parquet file for the
// startup recovery scan to re-queue.
func writeParquet[T any](tableName string, rows []T, tempDir string) (string, error) {
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	fileName := fmt.Sprintf("%s_%d.parquet", tableName, time.Now().UnixNano())
	filePath := filepath.Join(tempDir, fileName)
	tempPath := filePath + ".tmp"

	f, err := os.Create(tempPath)
	if err != nil {
		return "", fmt.Errorf("create parquet file: %w", err)
	}

	writer := parquet.NewGenericWriter[T](f)

	if _, err := writer.Write(rows); err != nil {
		f.Close()
		os.Remove(tempPath)
		return "", fmt.Errorf("write parquet rows: %w", err)
	}

	if err := writer.Close(); err != nil {
		f.Close()
		os.Remove(tempPath)
		return "", fmt.Errorf("close parquet writer: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tempPath)
		return "", fmt.Errorf("close parquet file: %w", err)
	}

	if err := os.Rename(tempPath, filePath); err != nil {
		os.Remove(tempPath)
		return "", fmt.Errorf("publish parquet file: %w", err)
	}

	return filePath, nil
}
