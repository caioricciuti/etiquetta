package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/spf13/cobra"
)

const (
	backupDatabaseName   = "etiquetta.duckdb"
	backupArchiveRoot    = "etiquetta-data"
	backupManifestName   = "etiquetta-backup-manifest.json"
	backupFormatVersion  = 1
	defaultBackupTimeout = 5 * time.Minute
)

var (
	backupOutput  string
	backupTimeout time.Duration
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Create a checksummed offline backup of all Etiquetta data",
	Long: `Creates a compressed archive containing the entire Etiquetta data directory.

The backup command obtains an exclusive DuckDB connection, checkpoints the
database, and closes it before reading any files. It refuses to continue while
the server (or another process using the database) is running. The archive
contains a manifest with SHA-256 checksums, and a checksum sidecar is written
next to the archive.

Stop Etiquetta before running this command. The output must be outside the data
directory and neither the archive nor its checksum file may already exist.`,
	Args: cobra.NoArgs,
	RunE: runBackup,
}

func init() {
	backupCmd.Flags().StringVarP(&backupOutput, "output", "o", "", "Backup archive path (default: ./etiquetta-backup-<UTC timestamp>.tar.gz)")
	backupCmd.Flags().DurationVar(&backupTimeout, "timeout", defaultBackupTimeout, "Maximum time for each offline database check and checkpoint")
}

type backupOptions struct {
	DataDir    string
	OutputPath string
	AppVersion string
	Timeout    time.Duration
	Now        func() time.Time
}

type backupResult struct {
	ArchivePath  string
	ChecksumPath string
	SHA256       string
	FileCount    int
	TotalBytes   int64
}

type backupDatabaseSnapshot struct {
	MigrationVersion int64                 `json:"migration_version"`
	Tables           []backupTableSnapshot `json:"tables"`
}

type backupTableSnapshot struct {
	Name            string `json:"name"`
	Present         bool   `json:"present"`
	RowCount        int64  `json:"row_count,omitempty"`
	LatestTimestamp *int64 `json:"latest_timestamp,omitempty"`
}

type backupManifest struct {
	FormatVersion int                    `json:"format_version"`
	Application   string                 `json:"application"`
	AppVersion    string                 `json:"app_version"`
	CreatedAt     time.Time              `json:"created_at"`
	ArchiveRoot   string                 `json:"archive_root"`
	Database      string                 `json:"database"`
	DatabaseState backupDatabaseSnapshot `json:"database_state"`
	EntryCount    int                    `json:"entry_count"`
	FileCount     int                    `json:"file_count"`
	TotalBytes    int64                  `json:"total_bytes"`
	Entries       []backupManifestEntry  `json:"entries"`
}

type backupManifestEntry struct {
	Path       string    `json:"path"`
	Type       string    `json:"type"`
	Size       int64     `json:"size,omitempty"`
	Mode       string    `json:"mode"`
	ModifiedAt time.Time `json:"modified_at"`
	SHA256     string    `json:"sha256,omitempty"`
	LinkTarget string    `json:"link_target,omitempty"`
}

func runBackup(cmd *cobra.Command, _ []string) error {
	if backupTimeout <= 0 {
		return errors.New("backup timeout must be greater than zero")
	}
	outputPath := backupOutput
	if outputPath == "" {
		outputPath = fmt.Sprintf("etiquetta-backup-%s.tar.gz", time.Now().UTC().Format("20060102T150405Z"))
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Offline backup: keep Etiquetta stopped until this command finishes.")
	result, err := createBackup(backupOptions{
		DataDir:    dataDir,
		OutputPath: outputPath,
		AppVersion: Version,
		Timeout:    backupTimeout,
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Backup complete: %s\n", result.ArchivePath)
	fmt.Fprintf(cmd.OutOrStdout(), "Checksum: %s\n", result.ChecksumPath)
	fmt.Fprintf(cmd.OutOrStdout(), "SHA-256: %s\n", result.SHA256)
	fmt.Fprintf(cmd.OutOrStdout(), "Archived %d files (%d bytes).\n", result.FileCount, result.TotalBytes)
	return nil
}

func createBackup(opts backupOptions) (result backupResult, retErr error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Timeout == 0 {
		opts.Timeout = defaultBackupTimeout
	}
	if opts.Timeout < 0 {
		return result, errors.New("backup timeout must be greater than zero")
	}
	createdAt := opts.Now().UTC()

	sourceDir, outputPath, checksumPath, err := validateBackupPaths(opts.DataDir, opts.OutputPath)
	if err != nil {
		return result, err
	}

	databasePath := filepath.Join(sourceDir, backupDatabaseName)
	checkpointCtx, cancelCheckpoint := context.WithTimeout(context.Background(), opts.Timeout)
	databaseState, err := checkpointInspectAndCloseDuckDB(checkpointCtx, databasePath)
	cancelCheckpoint()
	if err != nil {
		return result, err
	}

	// The temporary files live beside the final files so publishing with a hard
	// link is atomic and cannot overwrite a file created by another process.
	outputDir := filepath.Dir(outputPath)
	archiveTemp, err := os.CreateTemp(outputDir, ".etiquetta-backup-*.tar.gz.tmp")
	if err != nil {
		return result, fmt.Errorf("create temporary backup archive: %w", err)
	}
	archiveTempPath := archiveTemp.Name()
	defer func() {
		archiveTemp.Close()
		os.Remove(archiveTempPath)
	}()
	if err := archiveTemp.Chmod(0600); err != nil {
		return result, fmt.Errorf("secure temporary backup archive: %w", err)
	}

	manifest := backupManifest{
		FormatVersion: backupFormatVersion,
		Application:   "etiquetta",
		AppVersion:    opts.AppVersion,
		CreatedAt:     createdAt,
		ArchiveRoot:   backupArchiveRoot,
		Database:      path.Join(backupArchiveRoot, backupDatabaseName),
		DatabaseState: databaseState,
	}
	if err := writeBackupArchive(sourceDir, archiveTemp, &manifest); err != nil {
		return result, err
	}
	if err := archiveTemp.Sync(); err != nil {
		return result, fmt.Errorf("sync backup archive: %w", err)
	}
	if err := archiveTemp.Close(); err != nil {
		return result, fmt.Errorf("close backup archive: %w", err)
	}

	// The database lock necessarily ends before the archive begins. Re-read and
	// hash the complete source tree before publishing so a service restart or
	// any other concurrent write cannot silently produce a mixed backup.
	if err := verifyBackupSource(sourceDir, &manifest); err != nil {
		return result, fmt.Errorf("source data did not remain stable during the offline backup: %w", err)
	}
	probeCtx, cancelProbe := context.WithTimeout(context.Background(), opts.Timeout)
	err = probeAndCloseDuckDB(probeCtx, databasePath)
	cancelProbe()
	if err != nil {
		return result, err
	}
	if err := verifyBackupFile(sourceDir, &manifest, path.Join(backupArchiveRoot, backupDatabaseName)); err != nil {
		return result, fmt.Errorf("database changed during the final offline check: %w", err)
	}

	archiveHash, err := sha256File(archiveTempPath)
	if err != nil {
		return result, fmt.Errorf("checksum backup archive: %w", err)
	}

	checksumTemp, err := os.CreateTemp(outputDir, ".etiquetta-backup-*.sha256.tmp")
	if err != nil {
		return result, fmt.Errorf("create temporary checksum: %w", err)
	}
	checksumTempPath := checksumTemp.Name()
	defer func() {
		checksumTemp.Close()
		os.Remove(checksumTempPath)
	}()
	if err := checksumTemp.Chmod(0600); err != nil {
		return result, fmt.Errorf("secure temporary checksum: %w", err)
	}
	if _, err := fmt.Fprintf(checksumTemp, "%s  %s\n", archiveHash, filepath.Base(outputPath)); err != nil {
		return result, fmt.Errorf("write checksum: %w", err)
	}
	if err := checksumTemp.Sync(); err != nil {
		return result, fmt.Errorf("sync checksum: %w", err)
	}
	if err := checksumTemp.Close(); err != nil {
		return result, fmt.Errorf("close checksum: %w", err)
	}

	if err := publishFileNoReplace(archiveTempPath, outputPath); err != nil {
		return result, fmt.Errorf("publish backup archive: %w", err)
	}
	publishedArchive := true
	defer func() {
		if retErr != nil && publishedArchive {
			removePublishedFileIfSame(archiveTempPath, outputPath)
		}
	}()

	if err := publishFileNoReplace(checksumTempPath, checksumPath); err != nil {
		return result, fmt.Errorf("publish backup checksum: %w", err)
	}
	publishedArchive = false

	return backupResult{
		ArchivePath:  outputPath,
		ChecksumPath: checksumPath,
		SHA256:       archiveHash,
		FileCount:    manifest.FileCount,
		TotalBytes:   manifest.TotalBytes,
	}, nil
}

func validateBackupPaths(dataDirectory, requestedOutput string) (sourceDir, outputPath, checksumPath string, err error) {
	if strings.TrimSpace(dataDirectory) == "" {
		return "", "", "", errors.New("data directory must not be empty")
	}
	if strings.TrimSpace(requestedOutput) == "" {
		return "", "", "", errors.New("backup output path must not be empty")
	}

	sourceAbs, err := filepath.Abs(filepath.Clean(dataDirectory))
	if err != nil {
		return "", "", "", fmt.Errorf("resolve data directory: %w", err)
	}
	sourceDir, err = filepath.EvalSymlinks(sourceAbs)
	if err != nil {
		return "", "", "", fmt.Errorf("resolve data directory symlinks: %w", err)
	}
	sourceInfo, err := os.Stat(sourceDir)
	if err != nil {
		return "", "", "", fmt.Errorf("inspect data directory: %w", err)
	}
	if !sourceInfo.IsDir() {
		return "", "", "", fmt.Errorf("data path is not a directory: %s", sourceDir)
	}

	databasePath := filepath.Join(sourceDir, backupDatabaseName)
	databaseInfo, err := os.Stat(databasePath)
	if err != nil {
		return "", "", "", fmt.Errorf("inspect DuckDB database %s: %w", databasePath, err)
	}
	if !databaseInfo.Mode().IsRegular() {
		return "", "", "", fmt.Errorf("DuckDB database is not a regular file: %s", databasePath)
	}

	requestedAbs, err := filepath.Abs(filepath.Clean(requestedOutput))
	if err != nil {
		return "", "", "", fmt.Errorf("resolve backup output path: %w", err)
	}
	outputParent, err := filepath.EvalSymlinks(filepath.Dir(requestedAbs))
	if err != nil {
		return "", "", "", fmt.Errorf("resolve backup output directory: %w", err)
	}
	parentInfo, err := os.Stat(outputParent)
	if err != nil {
		return "", "", "", fmt.Errorf("inspect backup output directory: %w", err)
	}
	if !parentInfo.IsDir() {
		return "", "", "", fmt.Errorf("backup output parent is not a directory: %s", outputParent)
	}
	outputPath = filepath.Join(outputParent, filepath.Base(requestedAbs))
	checksumPath = outputPath + ".sha256"

	if isPathWithin(sourceDir, outputPath) {
		return "", "", "", fmt.Errorf("backup output must be outside the data directory: %s", outputPath)
	}
	for _, candidate := range []string{outputPath, checksumPath} {
		if _, statErr := os.Lstat(candidate); statErr == nil {
			return "", "", "", fmt.Errorf("refusing to overwrite existing file: %s", candidate)
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			return "", "", "", fmt.Errorf("inspect backup output %s: %w", candidate, statErr)
		}
	}

	return sourceDir, outputPath, checksumPath, nil
}

func isPathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func checkpointInspectAndCloseDuckDB(ctx context.Context, databasePath string) (snapshot backupDatabaseSnapshot, retErr error) {
	conn, err := sql.Open("duckdb", databasePath)
	if err != nil {
		return snapshot, offlineDatabaseError(err)
	}
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	defer func() {
		if closeErr := conn.Close(); retErr == nil && closeErr != nil {
			retErr = fmt.Errorf("close DuckDB before backup: %w", closeErr)
		}
	}()

	if err := conn.PingContext(ctx); err != nil {
		return snapshot, offlineDatabaseError(err)
	}
	if _, err := conn.ExecContext(ctx, "CHECKPOINT"); err != nil {
		return snapshot, fmt.Errorf("checkpoint DuckDB before backup: %w", err)
	}
	snapshot, err = inspectBackupDatabase(ctx, conn)
	if err != nil {
		return snapshot, fmt.Errorf("inspect checkpointed DuckDB: %w", err)
	}
	return snapshot, nil
}

func probeAndCloseDuckDB(ctx context.Context, databasePath string) error {
	conn, err := sql.Open("duckdb", databasePath)
	if err != nil {
		return offlineDatabaseError(err)
	}
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	if err := conn.PingContext(ctx); err != nil {
		conn.Close()
		return offlineDatabaseError(err)
	}
	if err := conn.Close(); err != nil {
		return fmt.Errorf("close DuckDB after final offline check: %w", err)
	}
	return nil
}

func offlineDatabaseError(err error) error {
	return fmt.Errorf("cannot acquire DuckDB for an offline backup; keep Etiquetta stopped and stop any other process using the database: %w", err)
}

func inspectBackupDatabase(ctx context.Context, conn *sql.DB) (backupDatabaseSnapshot, error) {
	snapshot := backupDatabaseSnapshot{Tables: make([]backupTableSnapshot, 0, len(backupLogicalTables))}

	migrationsPresent, err := backupTableExists(ctx, conn, "migrations")
	if err != nil {
		return snapshot, err
	}
	if migrationsPresent {
		if err := conn.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM migrations").Scan(&snapshot.MigrationVersion); err != nil {
			return snapshot, fmt.Errorf("read migration version: %w", err)
		}
	}

	for _, table := range backupLogicalTables {
		tableSnapshot := backupTableSnapshot{Name: table.name}
		present, err := backupTableExists(ctx, conn, table.name)
		if err != nil {
			return snapshot, err
		}
		tableSnapshot.Present = present
		if present {
			var latest sql.NullInt64
			query := fmt.Sprintf("SELECT COUNT(*), MAX(%s) FROM %s", quoteDuckDBIdentifier(table.timestampColumn), quoteDuckDBIdentifier(table.name))
			if err := conn.QueryRowContext(ctx, query).Scan(&tableSnapshot.RowCount, &latest); err != nil {
				return snapshot, fmt.Errorf("inspect table %s: %w", table.name, err)
			}
			if latest.Valid {
				value := latest.Int64
				tableSnapshot.LatestTimestamp = &value
			}
		}
		snapshot.Tables = append(snapshot.Tables, tableSnapshot)
	}
	return snapshot, nil
}

var backupLogicalTables = []struct {
	name            string
	timestampColumn string
}{
	{name: "events", timestampColumn: "timestamp"},
	{name: "performance", timestampColumn: "timestamp"},
	{name: "errors", timestampColumn: "timestamp"},
	{name: "visitor_sessions", timestampColumn: "start_time"},
	{name: "consent_records", timestampColumn: "timestamp"},
	{name: "session_recordings", timestampColumn: "start_time"},
	{name: "domains", timestampColumn: "created_at"},
	{name: "users", timestampColumn: "created_at"},
}

func backupTableExists(ctx context.Context, conn *sql.DB, tableName string) (bool, error) {
	var exists bool
	err := conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = 'main' AND table_name = ?
		)
	`, tableName).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("look up table %s: %w", tableName, err)
	}
	return exists, nil
}

func quoteDuckDBIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func writeBackupArchive(sourceDir string, destination io.Writer, manifest *backupManifest) error {
	gzipWriter := gzip.NewWriter(destination)
	tarWriter := tar.NewWriter(gzipWriter)

	walkErr := filepath.WalkDir(sourceDir, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		info, err := os.Lstat(filePath)
		if err != nil {
			return fmt.Errorf("inspect %s: %w", filePath, err)
		}

		relative, err := filepath.Rel(sourceDir, filePath)
		if err != nil {
			return fmt.Errorf("make archive path for %s: %w", filePath, err)
		}
		archiveName := backupArchiveRoot
		if relative != "." {
			archiveName = path.Join(backupArchiveRoot, filepath.ToSlash(relative))
		}

		entryType := ""
		linkTarget := ""
		switch {
		case info.IsDir():
			entryType = "directory"
		case info.Mode().IsRegular():
			entryType = "file"
		case info.Mode()&os.ModeSymlink != 0:
			entryType = "symlink"
			linkTarget, err = os.Readlink(filePath)
			if err != nil {
				return fmt.Errorf("read symbolic link %s: %w", filePath, err)
			}
		default:
			return fmt.Errorf("refusing to archive unsupported special file %s (%s)", filePath, info.Mode())
		}

		header, err := tar.FileInfoHeader(info, linkTarget)
		if err != nil {
			return fmt.Errorf("create archive header for %s: %w", filePath, err)
		}
		header.Name = archiveName
		if info.IsDir() {
			header.Name += "/"
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("write archive header for %s: %w", filePath, err)
		}

		manifestEntry := backupManifestEntry{
			Path:       archiveName,
			Type:       entryType,
			Mode:       info.Mode().String(),
			ModifiedAt: info.ModTime().UTC(),
			LinkTarget: linkTarget,
		}

		if info.Mode().IsRegular() {
			checksum, size, err := copyRegularFileToArchive(tarWriter, filePath, info)
			if err != nil {
				return err
			}
			manifestEntry.Size = size
			manifestEntry.SHA256 = checksum
			manifest.FileCount++
			manifest.TotalBytes += size
		}

		manifest.Entries = append(manifest.Entries, manifestEntry)
		return nil
	})

	if walkErr == nil {
		sort.Slice(manifest.Entries, func(i, j int) bool {
			return manifest.Entries[i].Path < manifest.Entries[j].Path
		})
		manifest.EntryCount = len(manifest.Entries)
		manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			walkErr = fmt.Errorf("encode backup manifest: %w", err)
		} else {
			manifestJSON = append(manifestJSON, '\n')
			header := &tar.Header{
				Name:     backupManifestName,
				Mode:     0600,
				Size:     int64(len(manifestJSON)),
				ModTime:  manifest.CreatedAt,
				Typeflag: tar.TypeReg,
				Format:   tar.FormatPAX,
			}
			if err := tarWriter.WriteHeader(header); err != nil {
				walkErr = fmt.Errorf("write backup manifest header: %w", err)
			} else if _, err := tarWriter.Write(manifestJSON); err != nil {
				walkErr = fmt.Errorf("write backup manifest: %w", err)
			}
		}
	}

	tarCloseErr := tarWriter.Close()
	gzipCloseErr := gzipWriter.Close()
	if walkErr != nil {
		return walkErr
	}
	if tarCloseErr != nil {
		return fmt.Errorf("finalize tar archive: %w", tarCloseErr)
	}
	if gzipCloseErr != nil {
		return fmt.Errorf("finalize compressed archive: %w", gzipCloseErr)
	}
	return nil
}

func copyRegularFileToArchive(destination io.Writer, filePath string, expected fs.FileInfo) (string, int64, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", 0, fmt.Errorf("open %s: %w", filePath, err)
	}
	defer file.Close()

	openedInfo, err := file.Stat()
	if err != nil {
		return "", 0, fmt.Errorf("inspect opened file %s: %w", filePath, err)
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(expected, openedInfo) {
		return "", 0, fmt.Errorf("file changed while preparing backup: %s", filePath)
	}

	hash := sha256.New()
	written, err := io.CopyN(io.MultiWriter(destination, hash), file, expected.Size())
	if err != nil {
		return "", 0, fmt.Errorf("archive %s: %w", filePath, err)
	}
	var extra [1]byte
	if count, readErr := file.Read(extra[:]); count != 0 || (readErr != nil && !errors.Is(readErr, io.EOF)) {
		return "", 0, fmt.Errorf("file grew while being archived: %s", filePath)
	}

	afterInfo, err := file.Stat()
	if err != nil {
		return "", 0, fmt.Errorf("reinspect archived file %s: %w", filePath, err)
	}
	pathInfo, err := os.Lstat(filePath)
	if err != nil {
		return "", 0, fmt.Errorf("reinspect archive source %s: %w", filePath, err)
	}
	if !os.SameFile(openedInfo, afterInfo) || !os.SameFile(openedInfo, pathInfo) ||
		afterInfo.Size() != expected.Size() || !afterInfo.ModTime().Equal(expected.ModTime()) {
		return "", 0, fmt.Errorf("file changed while being archived: %s", filePath)
	}

	return hex.EncodeToString(hash.Sum(nil)), written, nil
}

func verifyBackupSource(sourceDir string, manifest *backupManifest) error {
	expectedEntries := make(map[string]backupManifestEntry, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		expectedEntries[entry.Path] = entry
	}
	seenEntries := make(map[string]struct{}, len(expectedEntries))

	err := filepath.WalkDir(sourceDir, func(filePath string, directoryEntry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(sourceDir, filePath)
		if err != nil {
			return fmt.Errorf("make verification path for %s: %w", filePath, err)
		}
		archiveName := backupArchiveRoot
		if relative != "." {
			archiveName = path.Join(backupArchiveRoot, filepath.ToSlash(relative))
		}
		expected, ok := expectedEntries[archiveName]
		if !ok {
			return fmt.Errorf("new source entry appeared during backup: %s", filePath)
		}
		if err := verifyBackupManifestEntry(filePath, expected); err != nil {
			return err
		}
		seenEntries[archiveName] = struct{}{}
		return nil
	})
	if err != nil {
		return err
	}
	if len(seenEntries) != len(expectedEntries) {
		missing := make([]string, 0, len(expectedEntries)-len(seenEntries))
		for archiveName := range expectedEntries {
			if _, ok := seenEntries[archiveName]; !ok {
				missing = append(missing, archiveName)
			}
		}
		sort.Strings(missing)
		return fmt.Errorf("source entries disappeared during backup: %s", strings.Join(missing, ", "))
	}
	return nil
}

func verifyBackupFile(sourceDir string, manifest *backupManifest, archiveName string) error {
	var expected *backupManifestEntry
	for index := range manifest.Entries {
		if manifest.Entries[index].Path == archiveName {
			expected = &manifest.Entries[index]
			break
		}
	}
	if expected == nil {
		return fmt.Errorf("manifest does not contain %s", archiveName)
	}
	prefix := backupArchiveRoot + "/"
	if !strings.HasPrefix(archiveName, prefix) {
		return fmt.Errorf("invalid archive path %s", archiveName)
	}
	relative := filepath.FromSlash(strings.TrimPrefix(archiveName, prefix))
	filePath := filepath.Join(sourceDir, relative)
	if !isPathWithin(sourceDir, filePath) {
		return fmt.Errorf("archive path escapes data directory: %s", archiveName)
	}
	return verifyBackupManifestEntry(filePath, *expected)
}

func verifyBackupManifestEntry(filePath string, expected backupManifestEntry) error {
	info, err := os.Lstat(filePath)
	if err != nil {
		return fmt.Errorf("reinspect %s: %w", filePath, err)
	}
	if info.Mode().String() != expected.Mode || !info.ModTime().UTC().Equal(expected.ModifiedAt) {
		return fmt.Errorf("source entry metadata changed during backup: %s", filePath)
	}

	switch expected.Type {
	case "directory":
		if !info.IsDir() {
			return fmt.Errorf("source entry type changed during backup: %s", filePath)
		}
	case "symlink":
		if info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("source entry type changed during backup: %s", filePath)
		}
		target, err := os.Readlink(filePath)
		if err != nil {
			return fmt.Errorf("reinspect symbolic link %s: %w", filePath, err)
		}
		if target != expected.LinkTarget {
			return fmt.Errorf("symbolic link changed during backup: %s", filePath)
		}
	case "file":
		if !info.Mode().IsRegular() {
			return fmt.Errorf("source entry type changed during backup: %s", filePath)
		}
		checksum, size, err := copyRegularFileToArchive(io.Discard, filePath, info)
		if err != nil {
			return err
		}
		if size != expected.Size || checksum != expected.SHA256 {
			return fmt.Errorf("source file contents changed during backup: %s", filePath)
		}
	default:
		return fmt.Errorf("manifest contains unsupported entry type %q for %s", expected.Type, filePath)
	}
	return nil
}

func sha256File(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func publishFileNoReplace(source, destination string) error {
	if err := os.Link(source, destination); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("refusing to overwrite existing file: %s", destination)
		}
		return err
	}
	return nil
}

func removePublishedFileIfSame(source, destination string) {
	sourceInfo, sourceErr := os.Stat(source)
	destinationInfo, destinationErr := os.Stat(destination)
	if sourceErr == nil && destinationErr == nil && os.SameFile(sourceInfo, destinationInfo) {
		_ = os.Remove(destination)
	}
}
