package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/caioricciuti/etiquetta/internal/auth"
	"github.com/caioricciuti/etiquetta/internal/database"
	"github.com/caioricciuti/etiquetta/internal/settings"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize Etiquetta with an interactive setup wizard",
	Long: `Runs an interactive setup wizard to configure Etiquetta.

This will:
  1. Create the data directory
  2. Initialize the database
  3. Create an admin user
  4. Generate a secure secret key
  5. Optionally configure MaxMind GeoIP`,
	Run: runInit,
}

func runInit(cmd *cobra.Command, args []string) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("===========================================")
	fmt.Println("  Etiquetta Setup Wizard")
	fmt.Println("===========================================")
	fmt.Println()

	// Check if data directory exists
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		fmt.Printf("Creating data directory: %s\n", dataDir)
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			log.Fatalf("Failed to create data directory: %v", err)
		}
	}

	// Check if database already exists
	dbPath := dataDir + "/etiquetta.duckdb"
	dbExists := false
	if _, err := os.Stat(dbPath); err == nil {
		dbExists = true
		fmt.Println("Database already exists.")
		fmt.Print("Do you want to continue? This will add settings but won't overwrite existing data. [y/N]: ")
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Setup cancelled.")
			return
		}
	}

	// Initialize database
	fmt.Println("\nInitializing database...")
	db, err := database.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Run migrations
	if err := db.Migrate(); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	fmt.Println("Database migrations complete.")

	// Initialize settings service
	settingsSvc := settings.New(db.Conn())

	// Get or generate secret key (preserve existing to avoid invalidating JWT sessions)
	secretKey, _ := settingsSvc.Get("secret_key")
	if secretKey == "" {
		secretKey = settings.GenerateSecretKey()
		settingsSvc.Set("secret_key", secretKey)
		fmt.Println("Generated secure secret key.")
	} else {
		fmt.Println("Secret key already exists, keeping existing.")
	}
	settingsSvc.SetMasterKey(secretKey)

	// Check if setup is already complete
	setupComplete, _ := settingsSvc.Get("setup_complete")
	if setupComplete == "true" && dbExists {
		fmt.Println("\nSetup was already completed previously.")
		fmt.Print("Do you want to create another admin user? [y/N]: ")
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("\nSetup complete! Run 'etiquetta serve' to start the server.")
			return
		}
	}

	// Create admin user
	fmt.Println("\n--- Admin User Setup ---")

	// Get email
	fmt.Print("Admin email: ")
	email, _ := reader.ReadString('\n')
	email = strings.TrimSpace(email)
	if email == "" || !strings.Contains(email, "@") {
		log.Fatal("Invalid email address")
	}

	// Get name
	fmt.Print("Admin name (optional): ")
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)

	// Get password
	fmt.Print("Admin password (min 8 characters): ")
	passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		log.Fatalf("Failed to read password: %v", err)
	}
	password := string(passwordBytes)

	if len(password) < 8 {
		log.Fatal("Password must be at least 8 characters")
	}

	// Confirm password
	fmt.Print("Confirm password: ")
	confirmBytes, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		log.Fatalf("Failed to read password: %v", err)
	}

	if password != string(confirmBytes) {
		log.Fatal("Passwords do not match")
	}

	// Hash password
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		log.Fatalf("Failed to hash password: %v", err)
	}

	// Create user
	userID := auth.GenerateID()
	now := time.Now().UnixMilli()

	_, err = db.Conn().Exec(
		"INSERT INTO users (id, email, password_hash, name, role, created_at, updated_at) VALUES (?, ?, ?, ?, 'admin', ?, ?)",
		userID, email, passwordHash, name, now, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			log.Fatal("A user with this email already exists")
		}
		log.Fatalf("Failed to create user: %v", err)
	}
	fmt.Println("Admin user created successfully.")

	// Mark setup as complete
	settingsSvc.Set("setup_complete", "true")

	// Optional: Configure MaxMind
	fmt.Println("\n--- GeoIP Configuration (Optional) ---")
	fmt.Println("MaxMind GeoIP provides country/city data for visitor locations.")
	fmt.Println("You can get free credentials at: https://www.maxmind.com/en/geolite2/signup")
	fmt.Print("\nDo you want to configure MaxMind GeoIP now? [y/N]: ")
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response == "y" || response == "yes" {
		fmt.Print("MaxMind Account ID: ")
		accountID, _ := reader.ReadString('\n')
		accountID = strings.TrimSpace(accountID)

		fmt.Print("MaxMind License Key: ")
		licenseKey, _ := reader.ReadString('\n')
		licenseKey = strings.TrimSpace(licenseKey)

		if accountID != "" && licenseKey != "" {
			settingsSvc.Set("maxmind_account_id", accountID)
			settingsSvc.Set("maxmind_license_key", licenseKey)
			fmt.Println("MaxMind credentials saved.")
			fmt.Println("Run 'etiquetta geoip download' to download the GeoIP database.")
		}
	}

	// Save listen address
	settingsSvc.Set("listen_addr", listenAddr)

	// Domain setup
	fmt.Println("\n--- Domain Configuration ---")
	fmt.Print("What domain will you track? (e.g., example.com): ")
	domainName, _ := reader.ReadString('\n')
	domainName = strings.TrimSpace(domainName)

	var siteID string
	if domainName != "" {
		// Clean domain (remove protocol if present)
		domainName = strings.TrimPrefix(domainName, "https://")
		domainName = strings.TrimPrefix(domainName, "http://")
		domainName = strings.TrimSuffix(domainName, "/")

		// Generate IDs
		domainID := auth.GenerateID()
		siteID = "site_" + generateRandomID()[:16]
		now := time.Now().UnixMilli()

		_, err = db.Conn().Exec(
			"INSERT INTO domains (id, name, domain, site_id, created_by, created_at, is_active) VALUES (?, ?, ?, ?, ?, ?, 1)",
			domainID, domainName, domainName, siteID, userID, now,
		)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint") {
				fmt.Println("Domain already exists, skipping.")
			} else {
				log.Printf("Warning: Failed to create domain: %v", err)
			}
		} else {
			fmt.Printf("Domain '%s' added successfully.\n", domainName)
		}
	}

	fmt.Println("\n===========================================")
	fmt.Println("  Setup Complete!")
	fmt.Println("===========================================")
	fmt.Println()
	fmt.Printf("Data directory: %s\n", dataDir)
	fmt.Printf("Admin email: %s\n", email)
	if domainName != "" {
		fmt.Printf("Domain: %s\n", domainName)
	}
	fmt.Println()

	if siteID != "" {
		fmt.Println("Add this tracking snippet to your website:")
		fmt.Println()
		fmt.Println("  <script")
		fmt.Println("    defer")
		fmt.Printf("    data-site=\"%s\"\n", siteID)
		fmt.Printf("    src=\"http://localhost%s/s.js\"\n", listenAddr)
		fmt.Println("  ></script>")
		fmt.Println()
	}

	fmt.Println("Next steps:")
	fmt.Println("  1. Run 'etiquetta serve' to start the server")
	fmt.Printf("  2. Open http://localhost%s in your browser\n", listenAddr)
	fmt.Println("  3. Log in with your admin credentials")
	fmt.Println()
}

// generateRandomID generates a random hex ID
func generateRandomID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
