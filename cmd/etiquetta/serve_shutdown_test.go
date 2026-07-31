package main

import (
	"strings"
	"testing"
)

func TestRunServeReturnsListenFailureAfterCleanup(t *testing.T) {
	// Pin the single-file DuckDB backend: this test is about surfacing a listener
	// failure, not storage, and must not reach for the DuckLake extensions.
	t.Setenv("ETIQUETTA_STORAGE", "duckdb")

	previousDataDir := dataDir
	previousListenAddr := listenAddr
	previousDetach := detach
	dataDir = t.TempDir()
	listenAddr = "invalid-listen-address"
	detach = false
	t.Cleanup(func() {
		dataDir = previousDataDir
		listenAddr = previousListenAddr
		detach = previousDetach
	})

	err := runServe(serveCmd, nil)
	if err == nil {
		t.Fatal("runServe() error = nil, want occupied-listener failure")
	}
	if !strings.Contains(err.Error(), "server stopped unexpectedly") {
		t.Fatalf("runServe() error = %v, want surfaced listener failure", err)
	}
}
