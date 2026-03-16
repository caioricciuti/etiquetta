package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/caioricciuti/etiquetta/internal/database"
	"github.com/caioricciuti/etiquetta/internal/geoip"
	"github.com/caioricciuti/etiquetta/internal/settings"
)

var geoipCmd = &cobra.Command{
	Use:   "geoip",
	Short: "Manage GeoIP database",
	Long:  `Commands for managing the MaxMind GeoIP database.`,
}

var geoipDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download the GeoIP database from MaxMind",
	Long: `Downloads the GeoLite2-City database from MaxMind.

Requires MaxMind account credentials to be configured.
You can get free credentials at: https://www.maxmind.com/en/geolite2/signup

Configure credentials via:
  - 'etiquetta init' command
  - Settings page in the web UI
  - 'etiquetta geoip configure' command`,
	Run: runGeoIPDownload,
}

var geoipStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show GeoIP database status",
	Run:   runGeoIPStatus,
}

var geoipConfigureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Configure MaxMind credentials",
	Run:   runGeoIPConfigure,
}

func init() {
	geoipCmd.AddCommand(geoipDownloadCmd)
	geoipCmd.AddCommand(geoipStatusCmd)
	geoipCmd.AddCommand(geoipConfigureCmd)
}

func runGeoIPDownload(cmd *cobra.Command, args []string) {
	db, settingsSvc := initSettingsService()
	defer db.Close()

	// Get credentials
	accountID, _ := settingsSvc.Get("maxmind_account_id")
	licenseKey, _ := settingsSvc.Get("maxmind_license_key")

	if accountID == "" || licenseKey == "" {
		log.Fatal("MaxMind credentials not configured. Run 'etiquetta geoip configure' first.")
	}

	geoipPath := settingsSvc.GetWithDefault("geoip_path", dataDir+"/GeoLite2-City.mmdb")

	fmt.Println("Downloading GeoIP database from MaxMind...")
	fmt.Printf("Destination: %s\n", geoipPath)

	downloader := geoip.NewDownloader(accountID, licenseKey, dataDir)
	if err := downloader.Download(); err != nil {
		log.Fatalf("Download failed: %v", err)
	}

	// Update last downloaded timestamp
	settingsSvc.Set("geoip_last_updated", time.Now().Format(time.RFC3339))

	fmt.Println("GeoIP database downloaded successfully!")
}

func runGeoIPStatus(cmd *cobra.Command, args []string) {
	db, settingsSvc := initSettingsService()
	defer db.Close()

	geoipPath := settingsSvc.GetWithDefault("geoip_path", dataDir+"/GeoLite2-City.mmdb")
	lastUpdated, _ := settingsSvc.Get("geoip_last_updated")
	autoUpdate := settingsSvc.GetBool("geoip_auto_update", false)
	accountID, _ := settingsSvc.Get("maxmind_account_id")

	fmt.Println("GeoIP Database Status")
	fmt.Println("=====================")
	fmt.Printf("Path: %s\n", geoipPath)

	// Check if file exists
	if info, err := os.Stat(geoipPath); err == nil {
		fmt.Printf("Status: Installed\n")
		fmt.Printf("File size: %.2f MB\n", float64(info.Size())/(1024*1024))
		fmt.Printf("File modified: %s\n", info.ModTime().Format("2006-01-02 15:04:05"))
	} else {
		fmt.Printf("Status: Not installed\n")
	}

	if lastUpdated != "" {
		fmt.Printf("Last downloaded: %s\n", lastUpdated)
	}

	fmt.Printf("Auto-update: %v\n", autoUpdate)

	if accountID != "" {
		fmt.Printf("MaxMind Account: Configured (ID: %s...)\n", accountID[:min(6, len(accountID))])
	} else {
		fmt.Printf("MaxMind Account: Not configured\n")
	}
}

func runGeoIPConfigure(cmd *cobra.Command, args []string) {
	db, settingsSvc := initSettingsService()
	defer db.Close()

	fmt.Println("MaxMind GeoIP Configuration")
	fmt.Println("===========================")
	fmt.Println("Get your free credentials at: https://www.maxmind.com/en/geolite2/signup")
	fmt.Println()

	var accountID, licenseKey string
	fmt.Print("Account ID: ")
	fmt.Scanln(&accountID)

	fmt.Print("License Key: ")
	fmt.Scanln(&licenseKey)

	if accountID == "" || licenseKey == "" {
		log.Fatal("Both Account ID and License Key are required")
	}

	settingsSvc.Set("maxmind_account_id", accountID)
	settingsSvc.Set("maxmind_license_key", licenseKey)

	fmt.Println("\nCredentials saved successfully!")
	fmt.Println("Run 'etiquetta geoip download' to download the database.")
}

func initSettingsService() (*database.DB, *settings.Service) {
	db, err := database.New(dataDir + "/etiquetta.duckdb")
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	settingsSvc := settings.New(db.Conn())

	// Get secret key for encryption
	secretKey, _ := settingsSvc.Get("secret_key")
	if secretKey != "" {
		settingsSvc.SetMasterKey(secretKey)
	}

	return db, settingsSvc
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
