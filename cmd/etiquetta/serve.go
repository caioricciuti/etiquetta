package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/caioricciuti/etiquetta/internal/alerts"
	"github.com/caioricciuti/etiquetta/internal/api"
	"github.com/caioricciuti/etiquetta/internal/bot"
	"github.com/caioricciuti/etiquetta/internal/buffer"
	"github.com/caioricciuti/etiquetta/internal/config"
	"github.com/caioricciuti/etiquetta/internal/connections"
	"github.com/caioricciuti/etiquetta/internal/connections/providers"
	"github.com/caioricciuti/etiquetta/internal/database"
	"github.com/caioricciuti/etiquetta/internal/ducklake"
	"github.com/caioricciuti/etiquetta/internal/enrichment"
	"github.com/caioricciuti/etiquetta/internal/licensing"
	"github.com/caioricciuti/etiquetta/internal/migrate"
	"github.com/caioricciuti/etiquetta/internal/replay"
	"github.com/caioricciuti/etiquetta/internal/rollup"
	"github.com/caioricciuti/etiquetta/internal/settings"
	"github.com/caioricciuti/etiquetta/ui"
)

var detach bool

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the Etiquetta server",
	Long:  `Starts the Etiquetta analytics server and begins accepting tracking data.`,
	RunE:  runServe,
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop a running Etiquetta server",
	Long:  `Stops a detached Etiquetta server by reading its PID file and sending SIGTERM.`,
	Run:   runStop,
}

func init() {
	serveCmd.Flags().BoolVar(&detach, "detach", false, "Run server in background (detached mode)")
}

func runStop(cmd *cobra.Command, args []string) {
	pidPath := filepath.Join(dataDir, "etiquetta.pid")

	pidBytes, err := os.ReadFile(pidPath)
	if err != nil {
		fmt.Println("No PID file found. Is Etiquetta running in detached mode?")
		os.Exit(1)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil {
		fmt.Printf("Invalid PID file: %s\n", pidPath)
		os.Remove(pidPath)
		os.Exit(1)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		fmt.Printf("Process %d not found.\n", pid)
		os.Remove(pidPath)
		os.Exit(1)
	}

	// Check if process is alive
	if err := process.Signal(syscall.Signal(0)); err != nil {
		fmt.Printf("Process %d is not running (stale PID file). Cleaning up.\n", pid)
		os.Remove(pidPath)
		return
	}

	fmt.Printf("Stopping Etiquetta (PID: %d)...\n", pid)
	if err := process.Signal(syscall.SIGTERM); err != nil {
		fmt.Printf("Failed to send SIGTERM: %v\n", err)
		os.Exit(1)
	}

	// Wait up to 10 seconds for graceful shutdown
	for i := 0; i < 20; i++ {
		time.Sleep(500 * time.Millisecond)
		if err := process.Signal(syscall.Signal(0)); err != nil {
			fmt.Println("Etiquetta stopped.")
			os.Remove(pidPath)
			return
		}
	}

	fmt.Println("Process did not stop in time. You may need to kill it manually.")
	fmt.Printf("  kill -9 %d\n", pid)
}

func runServe(cmd *cobra.Command, args []string) error {
	// Handle detach mode - fork to background
	if detach {
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			log.Fatalf("Failed to create data directory: %v", err)
		}

		// Build command without -d flag
		execPath, err := os.Executable()
		if err != nil {
			log.Fatalf("Failed to get executable path: %v", err)
		}

		cmdArgs := []string{"serve", "-d=false", "--data", dataDir, "--listen", listenAddr}
		child := exec.Command(execPath, cmdArgs...)

		// Redirect output to log file
		logPath := filepath.Join(dataDir, "etiquetta.log")
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Fatalf("Failed to open log file: %v", err)
		}
		child.Stdout = logFile
		child.Stderr = logFile

		// Start detached process
		if err := child.Start(); err != nil {
			log.Fatalf("Failed to start background process: %v", err)
		}

		// Write PID file
		pidPath := filepath.Join(dataDir, "etiquetta.pid")
		if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", child.Process.Pid)), 0644); err != nil {
			log.Printf("Warning: Failed to write PID file: %v", err)
		}

		fmt.Printf("Etiquetta started in background (PID: %d)\n", child.Process.Pid)
		fmt.Printf("Log file: %s\n", logPath)
		fmt.Printf("PID file: %s\n", pidPath)
		return nil
	}

	// Ensure data directory exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	// Check if SQLite migration is needed
	sqlitePath, needsMigration := database.NeedsSQLiteMigration(dataDir)

	// Initialize DuckDB database. DuckLake storage is opt-in via
	// ETIQUETTA_STORAGE=ducklake: the connection attaches a DuckLake catalog and
	// routes the append-only event tables to it (metadata stays in DuckDB).
	duckdbPath := dataDir + "/etiquetta.duckdb"
	useDuckLake := os.Getenv("ETIQUETTA_STORAGE") == "ducklake"
	var db *database.DB
	var err error
	if useDuckLake {
		lakeDir := filepath.Join(dataDir, "lake")
		if err := os.MkdirAll(filepath.Join(lakeDir, "data"), 0o755); err != nil {
			log.Fatalf("Failed to create lake directory: %v", err)
		}
		// First transition into DuckLake drops the event tables from the main
		// database, so snapshot it once beforehand as a rollback point.
		if _, statErr := os.Stat(filepath.Join(lakeDir, "catalog.ducklake")); os.IsNotExist(statErr) {
			backup := duckdbPath + ".pre-ducklake"
			if _, err := os.Stat(backup); os.IsNotExist(err) {
				if err := copyFileAtomic(duckdbPath, backup); err != nil {
					log.Fatalf("Failed to back up database before DuckLake migration: %v", err)
				}
				log.Printf("Backed up database to %s before enabling DuckLake", backup)
			}
		}
		db, err = database.NewWithLake(duckdbPath, database.LakeOptions{
			CatalogPath: filepath.Join(lakeDir, "catalog.ducklake"),
			DataPath:    filepath.Join(lakeDir, "data"),
		})
	} else {
		db, err = database.New(duckdbPath)
	}
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Run DuckDB migrations (create tables in the main database).
	if err := db.Migrate(); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Move the event tables into the lake once the schema exists. Idempotent and
	// crash-safe; the main tables are dropped only after a verified copy.
	if useDuckLake {
		if err := ducklake.Lakeify(db.Conn(), ducklake.EventTables); err != nil {
			log.Fatalf("Failed to migrate event tables into DuckLake: %v", err)
		}
		log.Printf("Storage: DuckLake catalog at %s", filepath.Join(dataDir, "lake"))
	}

	// Migrate data from SQLite if needed
	if needsMigration {
		log.Printf("Found existing SQLite database at %s, migrating to DuckDB...", sqlitePath)
		if err := database.MigrateSQLite(db.Conn(), sqlitePath); err != nil {
			log.Fatalf("SQLite migration failed: %v", err)
		}
	}

	// Initialize settings service
	settingsSvc := settings.New(db.Conn())

	// Get or generate secret key
	secretKey, _ := settingsSvc.Get("secret_key")
	if secretKey == "" {
		secretKey = settings.GenerateSecretKey()
		settingsSvc.Set("secret_key", secretKey)
		log.Println("Generated new secret key")
	}
	settingsSvc.SetMasterKey(secretKey)

	// Wire Google Ads provider credentials from settings
	if gads, ok := providers.Get("google_ads"); ok {
		if g, ok := gads.(*providers.GoogleAds); ok {
			g.GetCredentials = func() (string, string, string, error) {
				clientID, _ := settingsSvc.Get("google_ads_client_id")
				clientSecret, _ := settingsSvc.Get("google_ads_client_secret")
				devToken, _ := settingsSvc.Get("google_ads_developer_token")
				if clientID == "" || clientSecret == "" {
					return "", "", "", fmt.Errorf("Google Ads credentials not configured")
				}
				return clientID, clientSecret, devToken, nil
			}
		}
	}

	// Wire Meta Ads provider credentials from settings
	if mads, ok := providers.Get("meta_ads"); ok {
		if m, ok := mads.(*providers.MetaAds); ok {
			m.GetCredentials = func() (string, string, error) {
				appID, _ := settingsSvc.Get("meta_ads_app_id")
				appSecret, _ := settingsSvc.Get("meta_ads_app_secret")
				if appID == "" || appSecret == "" {
					return "", "", fmt.Errorf("Meta Ads credentials not configured")
				}
				return appID, appSecret, nil
			}
		}
	}

	// Wire Microsoft Ads provider credentials from settings
	if msads, ok := providers.Get("microsoft_ads"); ok {
		if ms, ok := msads.(*providers.MicrosoftAds); ok {
			ms.GetCredentials = func() (string, string, string, error) {
				clientID, _ := settingsSvc.Get("microsoft_ads_client_id")
				clientSecret, _ := settingsSvc.Get("microsoft_ads_client_secret")
				devToken, _ := settingsSvc.Get("microsoft_ads_developer_token")
				if clientID == "" || clientSecret == "" {
					return "", "", "", fmt.Errorf("Microsoft Ads credentials not configured")
				}
				return clientID, clientSecret, devToken, nil
			}
		}
	}

	// Load settings into config
	geoipPath := settingsSvc.GetWithDefault("geoip_path", dataDir+"/GeoLite2-City.mmdb")
	allowedOrigins := settingsSvc.GetWithDefault("allowed_origins", "*")

	// Build config from settings and flags
	cfg := &config.Config{
		ListenAddr:            listenAddr,
		DataDir:               dataDir,
		GeoIPPath:             geoipPath,
		SessionTimeoutMinutes: settingsSvc.GetInt("session_timeout_minutes", 30),
		TrackPerformance:      settingsSvc.GetBool("track_performance", true),
		TrackErrors:           settingsSvc.GetBool("track_errors", true),
		RespectDNT:            settingsSvc.GetBool("respect_dnt", true),
		AllowedOrigins:        []string{allowedOrigins},
		SecretKey:             secretKey,
	}

	// Initialize enrichment service
	enricher := enrichment.New(cfg.GeoIPPath)

	// Initialize license manager
	licenseManager := licensing.NewManager(cfg.DataDir + "/license.json")

	// DuckLake storage (opt-in via ETIQUETTA_STORAGE=ducklake). Metadata stays in
	// Initialize buffer manager
	bufferCfg := buffer.DefaultConfig(duckdbPath, dataDir)
	bufferMgr := buffer.NewBufferManager(db.Conn(), bufferCfg)

	// Initialize migrate manager
	migrateStore := migrate.NewStore(db.Conn())
	migrateManager := migrate.NewJobManager(migrateStore, bufferMgr, dataDir)

	// Initialize compaction (runs daily). In DuckLake mode the event data lives
	// in the lake (self-managing parquet), so run DuckLake maintenance instead of
	// the DuckDB-space compactor.
	compactor := buffer.NewCompactor(db.Conn(), bufferMgr)
	compactCtx, compactCancel := context.WithCancel(context.Background())
	if useDuckLake {
		go func() {
			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if err := ducklake.Compact(db.Conn(), "lake", 30); err != nil {
						log.Printf("DuckLake maintenance: %v", err)
					}
				case <-compactCtx.Done():
					return
				}
			}
		}()
	} else {
		compactor.StartSchedule(compactCtx, bufferCfg.CompactHour)
	}

	// Initialize connections store and sync manager
	connStore := connections.NewStore(db, secretKey)
	syncManager := connections.NewSyncManager(connStore, 1*time.Hour)
	syncManager.Start()

	// Initialize replay store
	replayStore := replay.NewStore(dataDir)

	// Get embedded UI filesystem
	uiDist, err := fs.Sub(ui.DistFS, "dist")
	if err != nil {
		log.Fatalf("Failed to access embedded UI: %v", err)
	}

	// Create router
	router, releaseStreams := api.NewRouterWithShutdown(db, enricher, licenseManager, cfg, uiDist, bufferMgr, connStore, syncManager, migrateManager, replayStore)

	// Shared mutex: compaction takes exclusive lock, all other writers to
	// compacted tables (events, performance, errors, visitor_sessions) take read lock.
	dbMu := bufferMgr.DBMu()

	// Start rollup refresher: pre-aggregates daily summaries for fast dashboards.
	rollupRefresher := rollup.NewRefresher(db.Conn(), dbMu, 10*time.Minute)
	rollupRefresher.Start()

	// Start data retention cleanup goroutine.
	retentionCtx, retentionCancel := context.WithCancel(context.Background())
	var retentionWG sync.WaitGroup
	retentionWG.Add(1)
	go func() {
		defer retentionWG.Done()
		dbMu.RLock()
		runDataRetention(db, licenseManager, settingsSvc, replayStore)
		dbMu.RUnlock()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				dbMu.RLock()
				runDataRetention(db, licenseManager, settingsSvc, replayStore)
				dbMu.RUnlock()
			case <-retentionCtx.Done():
				return
			}
		}
	}()

	// Start bot batch analysis (every 15 minutes)
	batchAnalyzer := bot.NewBatchAnalyzer(db.Conn(), 15*time.Minute, dbMu)
	var batchWG sync.WaitGroup
	batchWG.Add(1)
	go func() {
		defer batchWG.Done()
		batchAnalyzer.Start()
	}()

	// Start alert evaluator (every 5 minutes)
	alertCtx, alertCancel := context.WithCancel(context.Background())
	var alertWG sync.WaitGroup
	alertWG.Add(1)
	go func() {
		defer alertWG.Done()
		alerts.NewEvaluator(db.Conn(), settingsSvc, dbMu).Start(alertCtx)
	}()

	// Start server
	server := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // disabled: SSE connections are long-lived; query timeouts enforce per-request limits
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("Etiquetta %s starting on %s", Version, cfg.ListenAddr)
	log.Printf("Data directory: %s", cfg.DataDir)
	log.Printf("Database: DuckDB at %s", duckdbPath)
	log.Printf("License: %s", licenseManager.GetTier())

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- server.ListenAndServe()
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	var serverErr error
	var shutdownErr error
	serverExited := false
	select {
	case sig := <-sigChan:
		log.Printf("Received %s, shutting down server...", sig)
	case serverErr = <-serverErrCh:
		serverExited = true
	}
	signal.Stop(sigChan)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Release long-lived SSE streams so Shutdown drains on in-flight requests
	// rather than blocking the full timeout on open dashboards.
	releaseStreams()

	// Stop accepting requests and wait for active handlers before stopping any
	// component that can receive their buffered writes.
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP graceful shutdown failed: %v", err)
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("HTTP graceful shutdown: %w", err))
		if closeErr := server.Close(); closeErr != nil {
			log.Printf("HTTP forced close failed: %v", closeErr)
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("HTTP forced close: %w", closeErr))
		}
	}

	// Cancel every background database user, then wait for the goroutines whose
	// lifecycle is owned here before draining buffers or closing DuckDB.
	alertCancel()
	retentionCancel()
	batchAnalyzer.Stop()
	compactCancel()
	rollupRefresher.Stop()
	syncManager.Stop()
	if err := migrateManager.Shutdown(shutdownCtx); err != nil {
		log.Printf("Import shutdown exceeded graceful deadline: %v", err)
		shutdownErr = errors.Join(shutdownErr, err)
		// Never close the buffer or database underneath an import goroutine.
		// Continue joining after the graceful deadline; the service manager may
		// still enforce its own outer process timeout.
		if joinErr := migrateManager.Shutdown(context.Background()); joinErr != nil {
			shutdownErr = errors.Join(shutdownErr, joinErr)
		}
	}

	alertWG.Wait()
	retentionWG.Wait()
	batchWG.Wait()
	compactor.Wait()

	log.Println("Flushing buffers...")
	if err := bufferMgr.Close(shutdownCtx); err != nil {
		log.Printf("Buffer shutdown warning: %v", err)
		shutdownErr = errors.Join(shutdownErr, err)
	}

	if !serverExited {
		serverErr = <-serverErrCh
	}
	if serverErr != nil && !errors.Is(serverErr, http.ErrServerClosed) {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("server stopped unexpectedly: %w", serverErr))
	}
	return shutdownErr
}

func runDataRetention(db *database.DB, lm *licensing.Manager, settingsSvc *settings.Service, replayStore *replay.Store) {
	tier := lm.GetTier()
	userChoice := settingsSvc.GetInt("data_retention_days", 180)

	var retentionDays int
	if tier == licensing.TierPro || tier == licensing.TierEnterprise {
		// Pro/Enterprise: honor user choice with no clamp
		if userChoice == -1 {
			retentionDays = 365 * 10
		} else {
			retentionDays = userChoice
		}
	} else {
		// Community: clamp to 180
		retentionDays = userChoice
		if retentionDays > 180 || retentionDays == -1 {
			retentionDays = 180
		}
	}

	if err := db.CleanupOldData(retentionDays); err != nil {
		log.Printf("Data retention cleanup failed: %v", err)
	} else {
		log.Printf("Data retention: cleaned up data older than %d days", retentionDays)
	}

	// Clean up old replay files
	cutoffDate := time.Now().AddDate(0, 0, -retentionDays).Format("2006-01-02")
	if err := replayStore.CleanupBefore(cutoffDate); err != nil {
		log.Printf("Replay cleanup failed: %v", err)
	}
}
