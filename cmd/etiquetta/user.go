package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/caioricciuti/etiquetta/internal/auth"
	"github.com/caioricciuti/etiquetta/internal/database"
)

var userCmd = &cobra.Command{
	Use:   "user",
	Short: "Manage users",
	Long:  `Commands for managing Etiquetta users.`,
}

var userCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new user",
	Run:   runUserCreate,
}

var userListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all users",
	Run:   runUserList,
}

var userDeleteCmd = &cobra.Command{
	Use:   "delete [email]",
	Short: "Delete a user by email",
	Args:  cobra.ExactArgs(1),
	Run:   runUserDelete,
}

var (
	userRole string
)

func init() {
	userCreateCmd.Flags().StringVarP(&userRole, "role", "r", "viewer", "User role (admin or viewer)")

	userCmd.AddCommand(userCreateCmd)
	userCmd.AddCommand(userListCmd)
	userCmd.AddCommand(userDeleteCmd)
}

func runUserCreate(cmd *cobra.Command, args []string) {
	db, err := database.New(dataDir + "/etiquetta.duckdb")
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	reader := bufio.NewReader(os.Stdin)

	// Get email
	fmt.Print("Email: ")
	email, _ := reader.ReadString('\n')
	email = strings.TrimSpace(email)
	if email == "" || !strings.Contains(email, "@") {
		log.Fatal("Invalid email address")
	}

	// Check if email already exists
	var existingID string
	err = db.Conn().QueryRow("SELECT id FROM users WHERE email = ?", email).Scan(&existingID)
	if err == nil {
		log.Fatal("A user with this email already exists")
	}

	// Get name
	fmt.Print("Name (optional): ")
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)

	// Validate role
	if userRole != "admin" && userRole != "viewer" {
		log.Fatal("Role must be 'admin' or 'viewer'")
	}

	// Get password
	fmt.Print("Password (min 8 characters): ")
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
		"INSERT INTO users (id, email, password_hash, name, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		userID, email, passwordHash, name, userRole, now, now,
	)
	if err != nil {
		log.Fatalf("Failed to create user: %v", err)
	}

	fmt.Printf("User created successfully: %s (%s)\n", email, userRole)
}

func runUserList(cmd *cobra.Command, args []string) {
	db, err := database.New(dataDir + "/etiquetta.duckdb")
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	rows, err := db.Conn().Query("SELECT id, email, name, role, created_at FROM users ORDER BY created_at DESC")
	if err != nil {
		log.Fatalf("Failed to query users: %v", err)
	}
	defer rows.Close()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tEMAIL\tNAME\tROLE\tCREATED")

	count := 0
	for rows.Next() {
		var id, email, name, role string
		var createdAt int64
		rows.Scan(&id, &email, &name, &role, &createdAt)

		created := time.UnixMilli(createdAt).Format("2006-01-02 15:04")
		if name == "" {
			name = "-"
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", id[:8], email, name, role, created)
		count++
	}
	w.Flush()

	fmt.Printf("\nTotal: %d user(s)\n", count)
}

func runUserDelete(cmd *cobra.Command, args []string) {
	email := args[0]

	db, err := database.New(dataDir + "/etiquetta.duckdb")
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Check if user exists
	var userID, name, role string
	err = db.Conn().QueryRow("SELECT id, name, role FROM users WHERE email = ?", email).Scan(&userID, &name, &role)
	if err != nil {
		log.Fatalf("User not found: %s", email)
	}

	// Confirm deletion
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("Are you sure you want to delete user '%s' (%s, %s)? [y/N]: ", email, name, role)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response != "y" && response != "yes" {
		fmt.Println("Cancelled.")
		return
	}

	// Delete user
	_, err = db.Conn().Exec("DELETE FROM users WHERE id = ?", userID)
	if err != nil {
		log.Fatalf("Failed to delete user: %v", err)
	}

	fmt.Printf("User deleted: %s\n", email)
}
