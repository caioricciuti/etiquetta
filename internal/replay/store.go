package replay

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Store manages session replay files on disk.
type Store struct {
	baseDir string
	mu      sync.Mutex
}

// NewStore creates a replay store rooted at dataDir/replays.
func NewStore(dataDir string) *Store {
	dir := filepath.Join(dataDir, "replays")
	os.MkdirAll(dir, 0755)
	return &Store{baseDir: dir}
}

// sessionPath returns the file path for a recording.
func (s *Store) sessionPath(domain, sessionID string) string {
	date := time.Now().Format("2006-01-02")
	return filepath.Join(s.baseDir, domain, date, sessionID+".json.gz")
}

// sessionGlob returns a glob matching all chunks for a session across dates.
func (s *Store) sessionGlob(domain, sessionID string) string {
	return filepath.Join(s.baseDir, domain, "*", sessionID+".json.gz")
}

// AppendEvents appends rrweb events to a session's gzip file.
// Events are appended as newline-delimited JSON.
func (s *Store) AppendEvents(domain, sessionID string, events []json.RawMessage) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.sessionPath(domain, sessionID)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return 0, fmt.Errorf("mkdir: %w", err)
	}

	// Read existing data if file exists
	var existing []byte
	if data, err := os.ReadFile(path); err == nil {
		existing = data
	}

	// Decompress existing events
	var allEvents []json.RawMessage
	if len(existing) > 0 {
		if decoded, err := decompressEvents(existing); err == nil {
			allEvents = decoded
		}
	}

	// Append new events
	allEvents = append(allEvents, events...)

	// Compress and write
	f, err := os.Create(path)
	if err != nil {
		return 0, fmt.Errorf("create: %w", err)
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	enc := json.NewEncoder(gz)
	for _, evt := range allEvents {
		if err := enc.Encode(evt); err != nil {
			gz.Close()
			return 0, fmt.Errorf("encode: %w", err)
		}
	}
	if err := gz.Close(); err != nil {
		return 0, fmt.Errorf("gzip close: %w", err)
	}

	stat, _ := f.Stat()
	return stat.Size(), nil
}

// ReadEvents reads all events for a session.
func (s *Store) ReadEvents(domain, sessionID string) ([]json.RawMessage, error) {
	matches, _ := filepath.Glob(s.sessionGlob(domain, sessionID))
	if len(matches) == 0 {
		return nil, fmt.Errorf("recording not found")
	}

	var allEvents []json.RawMessage
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		events, err := decompressEvents(data)
		if err != nil {
			continue
		}
		allEvents = append(allEvents, events...)
	}
	return allEvents, nil
}

// Delete removes a session recording from disk.
func (s *Store) Delete(domain, sessionID string) error {
	matches, _ := filepath.Glob(s.sessionGlob(domain, sessionID))
	for _, path := range matches {
		os.Remove(path)
	}
	return nil
}

// DiskUsageBytes returns total disk usage of replays directory.
func (s *Store) DiskUsageBytes() (int64, error) {
	var total int64
	filepath.Walk(s.baseDir, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total, nil
}

// CleanupBefore removes replay files for recordings older than the given date dir.
func (s *Store) CleanupBefore(cutoffDate string) error {
	domains, _ := os.ReadDir(s.baseDir)
	for _, d := range domains {
		if !d.IsDir() {
			continue
		}
		domainPath := filepath.Join(s.baseDir, d.Name())
		dates, _ := os.ReadDir(domainPath)
		for _, dateDir := range dates {
			if !dateDir.IsDir() {
				continue
			}
			if dateDir.Name() < cutoffDate {
				os.RemoveAll(filepath.Join(domainPath, dateDir.Name()))
			}
		}
	}
	return nil
}

func decompressEvents(data []byte) ([]json.RawMessage, error) {
	r, err := gzip.NewReader(io.NopCloser(newBytesReader(data)))
	if err != nil {
		return nil, err
	}
	defer r.Close()

	var events []json.RawMessage
	dec := json.NewDecoder(r)
	for {
		var evt json.RawMessage
		if err := dec.Decode(&evt); err != nil {
			if err == io.EOF {
				break
			}
			return events, nil // return what we have
		}
		events = append(events, evt)
	}
	return events, nil
}

type bytesReader struct {
	data []byte
	pos  int
}

func newBytesReader(data []byte) *bytesReader {
	return &bytesReader{data: data}
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
