package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestChecksumForAsset(t *testing.T) {
	want := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	data := []byte(want + "  etiquetta-linux-amd64\n")

	got, err := checksumForAsset(data, "etiquetta-linux-amd64")
	if err != nil {
		t.Fatalf("checksumForAsset() error = %v", err)
	}
	if got != want {
		t.Fatalf("checksumForAsset() = %q, want %q", got, want)
	}
}

func TestChecksumForAssetRejectsInvalidHash(t *testing.T) {
	_, err := checksumForAsset([]byte("not-a-hash  etiquetta-linux-amd64\n"), "etiquetta-linux-amd64")
	if err == nil {
		t.Fatal("checksumForAsset() accepted an invalid SHA-256")
	}
}

func TestCopyFileAtomicPreservesContentAndMode(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "etiquetta")
	dst := filepath.Join(dir, "etiquetta.previous")
	content := []byte("known-good-binary")
	if err := os.WriteFile(src, content, 0750); err != nil {
		t.Fatal(err)
	}

	if err := copyFileAtomic(src, dst); err != nil {
		t.Fatalf("copyFileAtomic() error = %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("copied content = %q, want %q", got, content)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if gotMode := info.Mode().Perm(); gotMode != 0750 {
		t.Fatalf("copied mode = %o, want 750", gotMode)
	}

	sum := sha256.Sum256(got)
	if hex.EncodeToString(sum[:]) == "" {
		t.Fatal("expected a non-empty checksum")
	}
}

func TestDownloadBinaryComputesChecksumAndEnforcesLimit(t *testing.T) {
	payload := []byte("verified release payload")

	dst, err := os.CreateTemp(t.TempDir(), "candidate-*")
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()

	got, err := writeVerifiedBinary(dst, bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatalf("downloadBinary() error = %v", err)
	}
	wantSum := sha256.Sum256(payload)
	want := hex.EncodeToString(wantSum[:])
	if got != want {
		t.Fatalf("downloadBinary() checksum = %q, want %q", got, want)
	}

	overflow, err := os.CreateTemp(t.TempDir(), "overflow-*")
	if err != nil {
		t.Fatal(err)
	}
	defer overflow.Close()
	if _, err := writeVerifiedBinary(overflow, bytes.NewReader(payload), int64(len(payload)-1)); err == nil {
		t.Fatal("downloadBinary() accepted a payload larger than its limit")
	}
}
