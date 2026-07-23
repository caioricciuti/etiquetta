package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateBackupIncludesAllStateAndVerifiableChecksums(t *testing.T) {
	root := t.TempDir()
	dataDirectory := filepath.Join(root, "data")
	outputDirectory := filepath.Join(root, "backups")
	mustMkdirAll(t, outputDirectory)
	createBackupTestDatabase(t, dataDirectory)

	stateFiles := map[string]string{
		"license.json":                                        `{"tier":"pro"}`,
		"GeoLite2-City.mmdb":                                  "geoip-state",
		filepath.Join("buffer_tmp", "batch"):                  "buffered-events",
		filepath.Join("replays", "example.com", "session.gz"): "replay-state",
		filepath.Join("migrate_tmp", "job.json"):              "migration-state",
		".hidden-state":                                       "hidden-state",
	}
	for relative, contents := range stateFiles {
		filePath := filepath.Join(dataDirectory, relative)
		mustMkdirAll(t, filepath.Dir(filePath))
		if err := os.WriteFile(filePath, []byte(contents), 0600); err != nil {
			t.Fatalf("write state file %s: %v", relative, err)
		}
	}
	if err := os.Symlink("license.json", filepath.Join(dataDirectory, "license-current.json")); err != nil {
		t.Fatalf("create state symlink: %v", err)
	}

	createdAt := time.Date(2026, time.July, 20, 12, 34, 56, 0, time.UTC)
	outputPath := filepath.Join(outputDirectory, "production.tar.gz")
	result, err := createBackup(backupOptions{
		DataDir:    dataDirectory,
		OutputPath: outputPath,
		AppVersion: "v-test",
		Now:        func() time.Time { return createdAt },
	})
	if err != nil {
		t.Fatalf("create backup: %v", err)
	}

	canonicalOutputDirectory, err := filepath.EvalSymlinks(outputDirectory)
	if err != nil {
		t.Fatalf("resolve expected output directory: %v", err)
	}
	wantOutputPath := filepath.Join(canonicalOutputDirectory, filepath.Base(outputPath))
	if result.ArchivePath != wantOutputPath {
		t.Fatalf("archive path = %q, want %q", result.ArchivePath, wantOutputPath)
	}
	if result.ChecksumPath != wantOutputPath+".sha256" {
		t.Fatalf("checksum path = %q, want %q", result.ChecksumPath, wantOutputPath+".sha256")
	}

	archiveEntries, archiveLinks := readBackupArchive(t, outputPath)
	for relative, expected := range stateFiles {
		archivePath := filepath.ToSlash(filepath.Join(backupArchiveRoot, relative))
		if actual := string(archiveEntries[archivePath]); actual != expected {
			t.Errorf("archive entry %s = %q, want %q", archivePath, actual, expected)
		}
	}
	if got := archiveLinks[pathForArchive("license-current.json")]; got != "license.json" {
		t.Errorf("archived symlink target = %q, want license.json", got)
	}
	if _, ok := archiveEntries[pathForArchive(backupDatabaseName)]; !ok {
		t.Fatalf("archive does not contain %s", backupDatabaseName)
	}

	manifestBytes, ok := archiveEntries[backupManifestName]
	if !ok {
		t.Fatalf("archive does not contain %s", backupManifestName)
	}
	var manifest backupManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("decode backup manifest: %v", err)
	}
	if manifest.FormatVersion != backupFormatVersion {
		t.Errorf("manifest format version = %d, want %d", manifest.FormatVersion, backupFormatVersion)
	}
	if manifest.AppVersion != "v-test" {
		t.Errorf("manifest app version = %q, want v-test", manifest.AppVersion)
	}
	if !manifest.CreatedAt.Equal(createdAt) {
		t.Errorf("manifest creation time = %s, want %s", manifest.CreatedAt, createdAt)
	}
	if manifest.Database != pathForArchive(backupDatabaseName) {
		t.Errorf("manifest database = %q, want %q", manifest.Database, pathForArchive(backupDatabaseName))
	}
	if manifest.DatabaseState.MigrationVersion != 30 {
		t.Errorf("manifest migration version = %d, want 30", manifest.DatabaseState.MigrationVersion)
	}
	eventsSnapshot := findBackupTableSnapshot(t, manifest.DatabaseState, "events")
	if !eventsSnapshot.Present || eventsSnapshot.RowCount != 2 || eventsSnapshot.LatestTimestamp == nil || *eventsSnapshot.LatestTimestamp != 2000 {
		t.Errorf("events logical snapshot = %+v, want present with 2 rows and latest timestamp 2000", eventsSnapshot)
	}
	if manifest.FileCount != result.FileCount || manifest.TotalBytes != result.TotalBytes {
		t.Errorf("manifest totals (%d, %d) do not match result (%d, %d)", manifest.FileCount, manifest.TotalBytes, result.FileCount, result.TotalBytes)
	}

	for _, entry := range manifest.Entries {
		if entry.Type != "file" {
			continue
		}
		contents, ok := archiveEntries[entry.Path]
		if !ok {
			t.Errorf("manifest file %s is missing from archive", entry.Path)
			continue
		}
		sum := sha256.Sum256(contents)
		if got := hex.EncodeToString(sum[:]); got != entry.SHA256 {
			t.Errorf("manifest checksum for %s = %s, want %s", entry.Path, entry.SHA256, got)
		}
		if entry.Size != int64(len(contents)) {
			t.Errorf("manifest size for %s = %d, want %d", entry.Path, entry.Size, len(contents))
		}
	}

	archiveBytes, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read backup archive: %v", err)
	}
	archiveSum := sha256.Sum256(archiveBytes)
	wantHash := hex.EncodeToString(archiveSum[:])
	if result.SHA256 != wantHash {
		t.Errorf("result checksum = %s, want %s", result.SHA256, wantHash)
	}
	checksumBytes, err := os.ReadFile(outputPath + ".sha256")
	if err != nil {
		t.Fatalf("read checksum sidecar: %v", err)
	}
	wantSidecar := fmt.Sprintf("%s  %s\n", wantHash, filepath.Base(outputPath))
	if string(checksumBytes) != wantSidecar {
		t.Errorf("checksum sidecar = %q, want %q", checksumBytes, wantSidecar)
	}

	for _, securePath := range []string{outputPath, outputPath + ".sha256"} {
		info, err := os.Stat(securePath)
		if err != nil {
			t.Fatalf("stat %s: %v", securePath, err)
		}
		if got := info.Mode().Perm(); got&0077 != 0 {
			t.Errorf("permissions for %s = %o, want no group/other access", securePath, got)
		}
	}

	// A backup must not mutate or invalidate the source database.
	conn, err := sql.Open("duckdb", filepath.Join(dataDirectory, backupDatabaseName))
	if err != nil {
		t.Fatalf("reopen source database: %v", err)
	}
	defer conn.Close()
	var value string
	if err := conn.QueryRow("SELECT value FROM backup_test WHERE id = 1").Scan(&value); err != nil {
		t.Fatalf("query source database after backup: %v", err)
	}
	if value != "preserved" {
		t.Errorf("source database value = %q, want preserved", value)
	}
}

func TestCreateBackupRefusesUnsafeOutputs(t *testing.T) {
	root := t.TempDir()
	dataDirectory := filepath.Join(root, "data")
	outputDirectory := filepath.Join(root, "backups")
	mustMkdirAll(t, outputDirectory)
	createBackupTestDatabase(t, dataDirectory)

	t.Run("inside data directory", func(t *testing.T) {
		outputPath := filepath.Join(dataDirectory, "unsafe.tar.gz")
		_, err := createBackup(backupOptions{DataDir: dataDirectory, OutputPath: outputPath})
		if err == nil || !strings.Contains(err.Error(), "outside the data directory") {
			t.Fatalf("error = %v, want output-overlap error", err)
		}
		if _, statErr := os.Stat(outputPath); !os.IsNotExist(statErr) {
			t.Fatalf("unsafe output was created: %v", statErr)
		}
	})

	t.Run("existing archive", func(t *testing.T) {
		outputPath := filepath.Join(outputDirectory, "existing.tar.gz")
		if err := os.WriteFile(outputPath, []byte("do-not-replace"), 0600); err != nil {
			t.Fatal(err)
		}
		_, err := createBackup(backupOptions{DataDir: dataDirectory, OutputPath: outputPath})
		if err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
			t.Fatalf("error = %v, want overwrite error", err)
		}
		contents, readErr := os.ReadFile(outputPath)
		if readErr != nil || string(contents) != "do-not-replace" {
			t.Fatalf("existing archive changed: contents=%q err=%v", contents, readErr)
		}
	})

	t.Run("existing checksum", func(t *testing.T) {
		outputPath := filepath.Join(outputDirectory, "checksum-collision.tar.gz")
		if err := os.WriteFile(outputPath+".sha256", []byte("do-not-replace"), 0600); err != nil {
			t.Fatal(err)
		}
		_, err := createBackup(backupOptions{DataDir: dataDirectory, OutputPath: outputPath})
		if err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
			t.Fatalf("error = %v, want overwrite error", err)
		}
		if _, statErr := os.Stat(outputPath); !os.IsNotExist(statErr) {
			t.Fatalf("archive was created despite checksum collision: %v", statErr)
		}
		contents, readErr := os.ReadFile(outputPath + ".sha256")
		if readErr != nil || string(contents) != "do-not-replace" {
			t.Fatalf("existing checksum changed: contents=%q err=%v", contents, readErr)
		}
	})
}

func TestCreateBackupRefusesDatabaseHeldByAnotherProcess(t *testing.T) {
	root := t.TempDir()
	dataDirectory := filepath.Join(root, "data")
	outputDirectory := filepath.Join(root, "backups")
	mustMkdirAll(t, outputDirectory)
	createBackupTestDatabase(t, dataDirectory)

	holder := exec.Command(os.Args[0], "-test.run=^TestBackupDuckDBHolderProcess$")
	holder.Env = append(os.Environ(), "ETIQUETTA_TEST_HOLD_DUCKDB="+filepath.Join(dataDirectory, backupDatabaseName))
	stdin, err := holder.StdinPipe()
	if err != nil {
		t.Fatalf("create holder stdin: %v", err)
	}
	stdout, err := holder.StdoutPipe()
	if err != nil {
		t.Fatalf("create holder stdout: %v", err)
	}
	holder.Stderr = os.Stderr
	if err := holder.Start(); err != nil {
		t.Fatalf("start DuckDB holder: %v", err)
	}
	defer func() {
		stdin.Close()
		if err := holder.Wait(); err != nil {
			t.Errorf("DuckDB holder failed: %v", err)
		}
	}()

	ready, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatalf("wait for DuckDB holder: %v", err)
	}
	if strings.TrimSpace(ready) != "ready" {
		t.Fatalf("unexpected DuckDB holder response: %q", ready)
	}

	outputPath := filepath.Join(outputDirectory, "must-not-exist.tar.gz")
	_, err = createBackup(backupOptions{DataDir: dataDirectory, OutputPath: outputPath})
	if err == nil || !strings.Contains(err.Error(), "offline backup") {
		t.Fatalf("error = %v, want live-database refusal", err)
	}
	if _, statErr := os.Stat(outputPath); !os.IsNotExist(statErr) {
		t.Fatalf("backup output was created while database was live: %v", statErr)
	}
}

func TestBackupDuckDBHolderProcess(t *testing.T) {
	databasePath := os.Getenv("ETIQUETTA_TEST_HOLD_DUCKDB")
	if databasePath == "" {
		return
	}
	conn, err := sql.Open("duckdb", databasePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := conn.Ping(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fmt.Println("ready")
	_, _ = io.Copy(io.Discard, os.Stdin)
	if err := conn.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func TestVerifyBackupSourceDetectsChanges(t *testing.T) {
	dataDirectory := t.TempDir()
	statePath := filepath.Join(dataDirectory, "state.json")
	if err := os.WriteFile(statePath, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	manifest := backupManifest{CreatedAt: time.Now().UTC()}
	var archive bytes.Buffer
	if err := writeBackupArchive(dataDirectory, &archive, &manifest); err != nil {
		t.Fatalf("create in-memory backup: %v", err)
	}
	if err := os.WriteFile(statePath, []byte("after!"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyBackupSource(dataDirectory, &manifest); err == nil {
		t.Fatal("source verification succeeded after source data changed")
	}
}

func createBackupTestDatabase(t *testing.T, dataDirectory string) {
	t.Helper()
	mustMkdirAll(t, dataDirectory)
	conn, err := sql.Open("duckdb", filepath.Join(dataDirectory, backupDatabaseName))
	if err != nil {
		t.Fatalf("open test DuckDB: %v", err)
	}
	if _, err := conn.Exec(`
		CREATE TABLE backup_test (id INTEGER PRIMARY KEY, value VARCHAR);
		INSERT INTO backup_test VALUES (1, 'preserved');
		CREATE TABLE migrations (version INTEGER PRIMARY KEY, applied_at BIGINT NOT NULL);
		INSERT INTO migrations VALUES (30, 1000);
		CREATE TABLE events (id VARCHAR PRIMARY KEY, timestamp BIGINT NOT NULL);
		INSERT INTO events VALUES ('one', 1000), ('two', 2000);
	`); err != nil {
		conn.Close()
		t.Fatalf("populate test DuckDB: %v", err)
	}
	if _, err := conn.Exec("CHECKPOINT"); err != nil {
		conn.Close()
		t.Fatalf("checkpoint test DuckDB: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close test DuckDB: %v", err)
	}
}

func findBackupTableSnapshot(t *testing.T, snapshot backupDatabaseSnapshot, name string) backupTableSnapshot {
	t.Helper()
	for _, table := range snapshot.Tables {
		if table.Name == name {
			return table
		}
	}
	t.Fatalf("logical snapshot does not contain table %s", name)
	return backupTableSnapshot{}
}

func readBackupArchive(t *testing.T, archivePath string) (map[string][]byte, map[string]string) {
	t.Helper()
	file, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("open backup archive: %v", err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("open gzip stream: %v", err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)

	entries := make(map[string][]byte)
	links := make(map[string]string)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read tar entry: %v", err)
		}
		switch header.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
			contents, err := io.ReadAll(tarReader)
			if err != nil {
				t.Fatalf("read tar contents for %s: %v", header.Name, err)
			}
			entries[header.Name] = contents
		case tar.TypeSymlink:
			links[header.Name] = header.Linkname
		}
	}
	return entries, links
}

func pathForArchive(relative string) string {
	return filepath.ToSlash(filepath.Join(backupArchiveRoot, relative))
}

func mustMkdirAll(t *testing.T, directory string) {
	t.Helper()
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatalf("create directory %s: %v", directory, err)
	}
}
