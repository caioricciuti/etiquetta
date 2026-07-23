package replay

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreRoundTripUsesExactSessionNames(t *testing.T) {
	t.Parallel()

	store := NewStore(t.TempDir())
	domain := "analytics.example.com"
	firstSession := "session[one]"
	secondSession := "session-two"

	if _, err := store.AppendEvents(domain, firstSession, []json.RawMessage{json.RawMessage(`{"id":1}`)}); err != nil {
		t.Fatalf("append first session: %v", err)
	}
	if _, err := store.AppendEvents(domain, secondSession, []json.RawMessage{json.RawMessage(`{"id":2}`)}); err != nil {
		t.Fatalf("append second session: %v", err)
	}
	if _, err := store.AppendEvents(domain, firstSession, []json.RawMessage{json.RawMessage(`{"id":3}`)}); err != nil {
		t.Fatalf("append first session again: %v", err)
	}

	events, err := store.ReadEvents(domain, firstSession)
	if err != nil {
		t.Fatalf("read first session: %v", err)
	}
	if len(events) != 2 || string(events[0]) != `{"id":1}` || string(events[1]) != `{"id":3}` {
		t.Fatalf("first session events = %s", events)
	}

	if err := store.Delete(domain, firstSession); err != nil {
		t.Fatalf("delete first session: %v", err)
	}
	if _, err := store.ReadEvents(domain, firstSession); err == nil {
		t.Fatal("deleted session is still readable")
	}
	secondEvents, err := store.ReadEvents(domain, secondSession)
	if err != nil || len(secondEvents) != 1 || string(secondEvents[0]) != `{"id":2}` {
		t.Fatalf("second session was affected by exact-name delete: events=%s err=%v", secondEvents, err)
	}
}

func TestStoreRejectsPathTraversalIdentifiers(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	store := NewStore(dataDir)
	events := []json.RawMessage{json.RawMessage(`{"safe":true}`)}
	tests := []struct {
		name      string
		domain    string
		sessionID string
	}{
		{"parent domain", "..", "safe-session"},
		{"domain traversal", "../../outside", "safe-session"},
		{"absolute domain", "/tmp/outside", "safe-session"},
		{"Windows domain traversal", `..\\outside`, "safe-session"},
		{"parent session", "example.com", ".."},
		{"session traversal", "example.com", "../../outside"},
		{"absolute session", "example.com", "/tmp/outside"},
		{"Windows session traversal", "example.com", `..\\outside`},
		{"NUL session", "example.com", "bad\x00session"},
		{"control session", "example.com", "bad\nsession"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := store.AppendEvents(tt.domain, tt.sessionID, events); !errors.Is(err, ErrInvalidIdentifier) {
				t.Fatalf("AppendEvents error = %v, want invalid identifier", err)
			}
			if _, err := store.ReadEvents(tt.domain, tt.sessionID); !errors.Is(err, ErrInvalidIdentifier) {
				t.Fatalf("ReadEvents error = %v, want invalid identifier", err)
			}
			if err := store.Delete(tt.domain, tt.sessionID); !errors.Is(err, ErrInvalidIdentifier) {
				t.Fatalf("Delete error = %v, want invalid identifier", err)
			}
		})
	}

	entries, err := os.ReadDir(filepath.Join(dataDir, "replays"))
	if err != nil {
		t.Fatalf("read replay root: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("replay root contains files after rejected writes: %v", entries)
	}
}

func TestStoreCannotFollowDomainSymlinkOutsideRoot(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	outsideDir := t.TempDir()
	store := NewStore(dataDir)
	link := filepath.Join(dataDir, "replays", "escape.example")
	if err := os.Symlink(outsideDir, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, err := store.AppendEvents("escape.example", "safe-session", []json.RawMessage{json.RawMessage(`{"id":1}`)})
	if err == nil {
		t.Fatal("AppendEvents followed a symlink outside the replay root")
	}
	entries, readErr := os.ReadDir(outsideDir)
	if readErr != nil {
		t.Fatalf("read outside directory: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("outside directory was modified: %v", entries)
	}
}

func TestAppendFailurePreservesPriorRecordingAndCleansTemp(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	store := NewStore(dataDir)
	domain := "example.com"
	sessionID := "session-one"
	if _, err := store.AppendEvents(domain, sessionID, []json.RawMessage{json.RawMessage(`{"id":1}`)}); err != nil {
		t.Fatalf("append original recording: %v", err)
	}

	if _, err := store.AppendEvents(domain, sessionID, []json.RawMessage{json.RawMessage(`{"broken"`)}); err == nil {
		t.Fatal("append with invalid event unexpectedly succeeded")
	}
	events, err := store.ReadEvents(domain, sessionID)
	if err != nil {
		t.Fatalf("read original recording after failed append: %v", err)
	}
	if len(events) != 1 || string(events[0]) != `{"id":1}` {
		t.Fatalf("failed append changed prior recording: %s", events)
	}

	dateDir := filepath.Join(dataDir, "replays", domain, time.Now().Format("2006-01-02"))
	entries, err := os.ReadDir(dateDir)
	if err != nil {
		t.Fatalf("read replay date directory: %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp-") {
			t.Fatalf("failed append left temp file %q", entry.Name())
		}
	}
}

func TestAppendEventsReturnsTotalSizeAcrossDateDirectories(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	store := NewStore(dataDir)
	domain := "example.com"
	sessionID := "midnight-session"

	// Simulate a session that started before midnight: a fragment already
	// exists in yesterday's date directory.
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	oldDir := filepath.Join(dataDir, "replays", domain, yesterday)
	if err := os.MkdirAll(oldDir, 0755); err != nil {
		t.Fatalf("create old date directory: %v", err)
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write([]byte("{\"id\":1}\n")); err != nil {
		t.Fatalf("write old fragment: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close old fragment: %v", err)
	}
	oldFile := filepath.Join(oldDir, sessionID+".json.gz")
	if err := os.WriteFile(oldFile, buf.Bytes(), 0600); err != nil {
		t.Fatalf("write old fragment file: %v", err)
	}

	size, err := store.AppendEvents(domain, sessionID, []json.RawMessage{json.RawMessage(`{"id":2}`)})
	if err != nil {
		t.Fatalf("append after midnight: %v", err)
	}

	todayFile := filepath.Join(dataDir, "replays", domain, time.Now().Format("2006-01-02"), sessionID+".json.gz")
	oldInfo, err := os.Stat(oldFile)
	if err != nil {
		t.Fatalf("stat old fragment: %v", err)
	}
	todayInfo, err := os.Stat(todayFile)
	if err != nil {
		t.Fatalf("stat today fragment: %v", err)
	}
	if want := oldInfo.Size() + todayInfo.Size(); size != want {
		t.Fatalf("AppendEvents size = %d, want total across fragments %d (old=%d today=%d)",
			size, want, oldInfo.Size(), todayInfo.Size())
	}

	events, err := store.ReadEvents(domain, sessionID)
	if err != nil {
		t.Fatalf("read session spanning midnight: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events across fragments = %d, want 2", len(events))
	}
}

func TestAppendQuarantinesCorruptExistingFile(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	store := NewStore(dataDir)
	domain := "example.com"
	sessionID := "corrupt-session"

	todayDir := filepath.Join(dataDir, "replays", domain, time.Now().Format("2006-01-02"))
	if err := os.MkdirAll(todayDir, 0755); err != nil {
		t.Fatalf("create date directory: %v", err)
	}
	corruptFile := filepath.Join(todayDir, sessionID+".json.gz")
	if err := os.WriteFile(corruptFile, []byte("not gzip at all"), 0600); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}

	size, err := store.AppendEvents(domain, sessionID, []json.RawMessage{json.RawMessage(`{"id":1}`)})
	if err != nil {
		t.Fatalf("append over corrupt file should recover, got: %v", err)
	}
	if size <= 0 {
		t.Fatalf("AppendEvents size = %d, want > 0", size)
	}

	if _, err := os.Stat(corruptFile + ".corrupt"); err != nil {
		t.Fatalf("corrupt file was not quarantined: %v", err)
	}
	events, err := store.ReadEvents(domain, sessionID)
	if err != nil {
		t.Fatalf("read recovered session: %v", err)
	}
	if len(events) != 1 || string(events[0]) != `{"id":1}` {
		t.Fatalf("recovered session events = %s", events)
	}
}

func TestCleanupBeforeContinuesPastUnreadableDomain(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("permission checks are bypassed for root")
	}

	dataDir := t.TempDir()
	store := NewStore(dataDir)
	replayRoot := filepath.Join(dataDir, "replays")

	// Sorts before the healthy domain, so its ReadDir failure happens first.
	brokenDomain := filepath.Join(replayRoot, "a-broken.example")
	if err := os.MkdirAll(brokenDomain, 0755); err != nil {
		t.Fatalf("create broken domain: %v", err)
	}
	expiredDir := filepath.Join(replayRoot, "b-healthy.example", "2025-12-31")
	if err := os.MkdirAll(expiredDir, 0755); err != nil {
		t.Fatalf("create expired directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(expiredDir, "old.json.gz"), []byte("old"), 0600); err != nil {
		t.Fatalf("write expired replay: %v", err)
	}

	if err := os.Chmod(brokenDomain, 0000); err != nil {
		t.Fatalf("chmod broken domain: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(brokenDomain, 0755) })

	err := store.CleanupBefore("2026-01-01")
	if err == nil {
		t.Fatal("CleanupBefore should report the unreadable domain")
	}
	if _, statErr := os.Stat(expiredDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expired directory in later domain was not removed: %v", statErr)
	}
}

func TestCleanupBeforeRejectsUnsafeCutoff(t *testing.T) {
	t.Parallel()

	store := NewStore(t.TempDir())
	for _, cutoff := range []string{"", "../2026-01-01", "2026-1-1", "not-a-date"} {
		if err := store.CleanupBefore(cutoff); err == nil {
			t.Fatalf("CleanupBefore(%q) unexpectedly succeeded", cutoff)
		}
	}
}

func TestCleanupBeforeRemovesOnlyExpiredDateDirectories(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	store := NewStore(dataDir)
	replayRoot := filepath.Join(dataDir, "replays")
	oldDir := filepath.Join(replayRoot, "example.com", "2025-12-31")
	currentDir := filepath.Join(replayRoot, "example.com", "2026-01-01")
	if err := os.MkdirAll(oldDir, 0755); err != nil {
		t.Fatalf("create old replay directory: %v", err)
	}
	if err := os.MkdirAll(currentDir, 0755); err != nil {
		t.Fatalf("create current replay directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "old.json.gz"), []byte("old"), 0600); err != nil {
		t.Fatalf("write old replay: %v", err)
	}
	if err := os.WriteFile(filepath.Join(currentDir, "current.json.gz"), []byte("current"), 0600); err != nil {
		t.Fatalf("write current replay: %v", err)
	}

	outsideDir := t.TempDir()
	sentinel := filepath.Join(outsideDir, "keep.txt")
	if err := os.WriteFile(sentinel, []byte("keep"), 0600); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(replayRoot, "example.com", "2025-01-01")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := store.CleanupBefore("2026-01-01"); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err := os.Stat(oldDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired replay directory still exists or stat failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(currentDir, "current.json.gz")); err != nil {
		t.Fatalf("cutoff-date replay was removed: %v", err)
	}
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != "keep" {
		t.Fatalf("cleanup escaped replay root: content=%q err=%v", got, err)
	}
}
