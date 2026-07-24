package replay

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// ErrInvalidIdentifier indicates that a domain or session identifier cannot
// safely be represented as a replay-store path component.
var ErrInvalidIdentifier = errors.New("invalid replay identifier")

// Store manages session replay files on disk.
type Store struct {
	baseDir string
	mu      sync.Mutex
}

// NewStore creates a replay store rooted at dataDir/replays.
func NewStore(dataDir string) *Store {
	dir := filepath.Join(dataDir, "replays")
	_ = os.MkdirAll(dir, 0755)
	if absoluteDir, err := filepath.Abs(dir); err == nil {
		dir = absoluteDir
	}
	return &Store{baseDir: dir}
}

// ValidateIdentifiers ensures untrusted replay identifiers cannot change the
// directory structure. It deliberately permits ordinary Unicode and hostname
// punctuation for compatibility, while rejecting path separators, dot
// segments, control bytes, and overlong filesystem components.
func ValidateIdentifiers(domain, sessionID string) error {
	if err := validatePathComponent("domain", domain, 253); err != nil {
		return err
	}
	// Leave room for the .json.gz suffix on filesystems with 255-byte names.
	if err := validatePathComponent("session ID", sessionID, 200); err != nil {
		return err
	}
	return nil
}

func validatePathComponent(kind, value string, maxBytes int) error {
	if value == "" || value == "." || value == ".." || len(value) > maxBytes || !utf8.ValidString(value) {
		return fmt.Errorf("%w: unsafe %s", ErrInvalidIdentifier, kind)
	}
	if strings.ContainsAny(value, "/\\\x00") {
		return fmt.Errorf("%w: unsafe %s", ErrInvalidIdentifier, kind)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return fmt.Errorf("%w: unsafe %s", ErrInvalidIdentifier, kind)
		}
	}
	return nil
}

func (s *Store) openRoot() (*os.Root, error) {
	root, err := os.OpenRoot(s.baseDir)
	if err != nil {
		return nil, fmt.Errorf("open replay root: %w", err)
	}
	return root, nil
}

func sessionPath(domain, date, sessionID string) string {
	return path.Join(domain, date, sessionID+".json.gz")
}

// rootMkdirAll is the Go 1.24-compatible equivalent of os.Root.MkdirAll. Every
// component is still resolved by os.Root, so a symlink cannot escape baseDir.
func rootMkdirAll(root *os.Root, name string, perm fs.FileMode) error {
	current := "."
	for _, component := range strings.Split(name, "/") {
		if component == "" || component == "." {
			continue
		}
		current = path.Join(current, component)
		if err := root.Mkdir(current, perm); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
		info, err := root.Stat(current)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("%s is not a directory", current)
		}
	}
	return nil
}

// rootRemoveAll is the Go 1.24-compatible equivalent of os.Root.RemoveAll.
// DirEntry.IsDir does not follow symlinks, and all removals remain root-bound.
func rootRemoveAll(root *os.Root, name string) error {
	entries, err := fs.ReadDir(root.FS(), name)
	if err == nil {
		for _, entry := range entries {
			child := path.Join(name, entry.Name())
			if entry.IsDir() {
				if err := rootRemoveAll(root, child); err != nil {
					return err
				}
				continue
			}
			if err := root.Remove(child); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := root.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func createReplayTemp(root *os.Root, dir, finalPath string) (string, *os.File, error) {
	for range 10 {
		var random [8]byte
		if _, err := rand.Read(random[:]); err != nil {
			return "", nil, fmt.Errorf("generate replay temp name: %w", err)
		}
		tempName := "." + path.Base(finalPath) + ".tmp-" + hex.EncodeToString(random[:])
		tempPath := path.Join(dir, tempName)
		file, err := root.OpenFile(tempPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			return tempPath, file, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", nil, err
		}
	}
	return "", nil, fmt.Errorf("could not allocate replay temp file")
}

func (s *Store) absolutePath(relative string) (string, error) {
	if !fs.ValidPath(relative) {
		return "", fmt.Errorf("invalid replay path")
	}
	fullPath := filepath.Join(s.baseDir, filepath.FromSlash(relative))
	relativeCheck, err := filepath.Rel(s.baseDir, fullPath)
	if err != nil || relativeCheck == ".." || strings.HasPrefix(relativeCheck, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("replay path escapes root")
	}
	return fullPath, nil
}

// publishReplayTemp atomically replaces the destination with a synced temp
// file in the same directory. os.Root.Rename was added in Go 1.25, while this
// project supports Go 1.24, so the rename uses pre-validated absolute paths;
// creation and cleanup remain protected by os.Root.
func (s *Store) publishReplayTemp(tempPath, finalPath string) error {
	tempAbsolute, err := s.absolutePath(tempPath)
	if err != nil {
		return err
	}
	finalAbsolute, err := s.absolutePath(finalPath)
	if err != nil {
		return err
	}
	if filepath.Dir(tempAbsolute) != filepath.Dir(finalAbsolute) {
		return fmt.Errorf("replay temp and destination are not in the same directory")
	}
	if err := os.Rename(tempAbsolute, finalAbsolute); err != nil {
		return fmt.Errorf("replace replay: %w", err)
	}
	return nil
}

// AppendEvents appends rrweb events to a session's gzip file.
// Events are appended as newline-delimited JSON.
func (s *Store) AppendEvents(domain, sessionID string, events []json.RawMessage) (int64, error) {
	if err := ValidateIdentifiers(domain, sessionID); err != nil {
		return 0, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	root, err := s.openRoot()
	if err != nil {
		return 0, err
	}
	defer root.Close()

	date := time.Now().Format("2006-01-02")
	dir := path.Join(domain, date)
	filePath := sessionPath(domain, date, sessionID)
	if err := rootMkdirAll(root, dir, 0755); err != nil {
		return 0, fmt.Errorf("mkdir replay path: %w", err)
	}

	var allEvents []json.RawMessage
	existing, readErr := fs.ReadFile(root.FS(), filePath)
	if readErr == nil && len(existing) > 0 {
		decoded, err := decompressEvents(existing)
		if err != nil {
			// Quarantine the undecodable file and start fresh so appends recover
			// instead of failing forever on the same corrupt file.
			absolute, pathErr := s.absolutePath(filePath)
			if pathErr != nil {
				return 0, pathErr
			}
			if renameErr := os.Rename(absolute, absolute+".corrupt"); renameErr != nil {
				return 0, fmt.Errorf("quarantine corrupt replay: %w", renameErr)
			}
			log.Printf("[replay] Quarantined corrupt replay file %s.corrupt: %v", filePath, err)
		} else {
			allEvents = decoded
		}
	} else if readErr != nil && !errors.Is(readErr, fs.ErrNotExist) {
		return 0, fmt.Errorf("read existing replay: %w", readErr)
	}
	allEvents = append(allEvents, events...)

	tempPath, f, err := createReplayTemp(root, dir, filePath)
	if err != nil {
		return 0, fmt.Errorf("create replay temp: %w", err)
	}
	tempPublished := false
	defer func() {
		if !tempPublished {
			_ = root.Remove(tempPath)
		}
	}()
	gz := gzip.NewWriter(f)
	enc := json.NewEncoder(gz)
	for _, evt := range allEvents {
		if err := enc.Encode(evt); err != nil {
			_ = gz.Close()
			_ = f.Close()
			return 0, fmt.Errorf("encode replay: %w", err)
		}
	}
	if err := gz.Close(); err != nil {
		_ = f.Close()
		return 0, fmt.Errorf("close replay gzip: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return 0, fmt.Errorf("sync replay: %w", err)
	}
	if err := f.Close(); err != nil {
		return 0, fmt.Errorf("close replay: %w", err)
	}
	if err := s.publishReplayTemp(tempPath, filePath); err != nil {
		return 0, err
	}
	tempPublished = true
	// A session can span midnight and end up with one file per date directory;
	// report the total size across all of them, not just today's fragment.
	files, err := sessionFiles(root, domain, sessionID)
	if err != nil {
		return 0, fmt.Errorf("stat replay: %w", err)
	}
	var total int64
	for _, sessionFile := range files {
		info, err := root.Stat(sessionFile)
		if err != nil {
			continue
		}
		total += info.Size()
	}
	return total, nil
}

// sessionFiles returns exact session filenames across date directories. It
// intentionally does not use filepath.Glob: session IDs are untrusted and glob
// metacharacters must never broaden a read or delete.
func sessionFiles(root *os.Root, domain, sessionID string) ([]string, error) {
	dates, err := fs.ReadDir(root.FS(), domain)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	files := make([]string, 0, len(dates))
	for _, dateEntry := range dates {
		if !dateEntry.IsDir() {
			continue
		}
		date := dateEntry.Name()
		if parsed, err := time.Parse("2006-01-02", date); err != nil || parsed.Format("2006-01-02") != date {
			continue
		}
		filePath := sessionPath(domain, date, sessionID)
		info, err := root.Stat(filePath)
		if err == nil && info.Mode().IsRegular() {
			files = append(files, filePath)
		}
	}
	return files, nil
}

// ReadEvents reads all events for a session.
func (s *Store) ReadEvents(domain, sessionID string) ([]json.RawMessage, error) {
	if err := ValidateIdentifiers(domain, sessionID); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	root, err := s.openRoot()
	if err != nil {
		return nil, err
	}
	defer root.Close()

	files, err := sessionFiles(root, domain, sessionID)
	if err != nil {
		return nil, fmt.Errorf("find recording: %w", err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("recording not found")
	}

	var allEvents []json.RawMessage
	for _, filePath := range files {
		data, err := fs.ReadFile(root.FS(), filePath)
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
	if err := ValidateIdentifiers(domain, sessionID); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	root, err := s.openRoot()
	if err != nil {
		return err
	}
	defer root.Close()

	files, err := sessionFiles(root, domain, sessionID)
	if err != nil {
		return fmt.Errorf("find recording: %w", err)
	}
	for _, filePath := range files {
		if err := root.Remove(filePath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove recording: %w", err)
		}
	}
	return nil
}

// DiskUsageBytes returns total disk usage of replays directory.
func (s *Store) DiskUsageBytes() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	root, err := s.openRoot()
	if err != nil {
		return 0, err
	}
	defer root.Close()

	var total int64
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type().IsRegular() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("walk replay root: %w", err)
	}
	return total, nil
}

// CleanupBefore removes replay files for recordings older than the given date dir.
func (s *Store) CleanupBefore(cutoffDate string) error {
	cutoff, err := time.Parse("2006-01-02", cutoffDate)
	if err != nil || cutoff.Format("2006-01-02") != cutoffDate {
		return fmt.Errorf("invalid replay cutoff date")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	root, err := s.openRoot()
	if err != nil {
		return err
	}
	defer root.Close()

	domains, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return fmt.Errorf("read replay root: %w", err)
	}
	var errs []error
	for _, domainEntry := range domains {
		if !domainEntry.IsDir() {
			continue
		}
		domain := domainEntry.Name()
		dates, err := fs.ReadDir(root.FS(), domain)
		if err != nil {
			// Keep retention working for the remaining domains; report at the end.
			log.Printf("[replay] Cleanup skipped domain %s: %v", domain, err)
			errs = append(errs, fmt.Errorf("read replay domain %s: %w", domain, err))
			continue
		}
		for _, dateEntry := range dates {
			if !dateEntry.IsDir() {
				continue
			}
			date, err := time.Parse("2006-01-02", dateEntry.Name())
			if err != nil || date.Format("2006-01-02") != dateEntry.Name() || !date.Before(cutoff) {
				continue
			}
			if err := rootRemoveAll(root, path.Join(domain, dateEntry.Name())); err != nil {
				log.Printf("[replay] Cleanup failed for %s/%s: %v", domain, dateEntry.Name(), err)
				errs = append(errs, fmt.Errorf("remove expired replay directory %s/%s: %w", domain, dateEntry.Name(), err))
			}
		}
	}
	return errors.Join(errs...)
}

func decompressEvents(data []byte) ([]json.RawMessage, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
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
			// Preserve any complete events from a partially written tail. This is
			// also compatible with replay files produced by older versions.
			return events, nil
		}
		events = append(events, evt)
	}
	return events, nil
}
