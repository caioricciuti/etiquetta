package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update Etiquetta to the latest version",
	Long:  `Downloads, verifies, and atomically installs the latest Etiquetta release.`,
	Run:   runUpdate,
}

const (
	maxReleaseMetadataSize = 4 << 20
	maxChecksumFileSize    = 1 << 20
	maxBinarySize          = 512 << 20
)

var updateHTTPClient = &http.Client{Timeout: 2 * time.Minute}

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func runUpdate(cmd *cobra.Command, args []string) {
	fmt.Println("Checking for updates...")

	// Get latest release info
	resp, err := updateHTTPClient.Get("https://api.github.com/repos/caioricciuti/etiquetta/releases/latest")
	if err != nil {
		fmt.Printf("Failed to check for updates: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		fmt.Printf("Failed to fetch release info: HTTP %d\n", resp.StatusCode)
		os.Exit(1)
	}

	var release githubRelease
	metadata, err := io.ReadAll(io.LimitReader(resp.Body, maxReleaseMetadataSize+1))
	if err != nil {
		fmt.Printf("Failed to read release info: %v\n", err)
		os.Exit(1)
	}
	if len(metadata) > maxReleaseMetadataSize {
		fmt.Println("Release metadata exceeds the allowed size.")
		os.Exit(1)
	}
	if err := json.Unmarshal(metadata, &release); err != nil {
		fmt.Printf("Failed to parse release info: %v\n", err)
		os.Exit(1)
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	currentVersion := strings.TrimPrefix(Version, "v")

	fmt.Printf("Current version: %s\n", Version)
	fmt.Printf("Latest version:  %s\n", release.TagName)

	if currentVersion == latestVersion {
		fmt.Println("Already running the latest version.")
		return
	}

	// Determine binary name for this platform
	binaryName := fmt.Sprintf("etiquetta-%s-%s", runtime.GOOS, runtime.GOARCH)

	// Find download URL
	var downloadURL, checksumsURL string
	for _, asset := range release.Assets {
		if asset.Name == binaryName {
			downloadURL = asset.BrowserDownloadURL
		}
		if asset.Name == "checksums.txt" {
			checksumsURL = asset.BrowserDownloadURL
		}
	}

	if downloadURL == "" {
		fmt.Printf("No binary available for %s/%s\n", runtime.GOOS, runtime.GOARCH)
		os.Exit(1)
	}
	if checksumsURL == "" {
		fmt.Println("Release has no checksums.txt; refusing an unverified update.")
		os.Exit(1)
	}

	checksumData, err := downloadBytes(checksumsURL, maxChecksumFileSize)
	if err != nil {
		fmt.Printf("Failed to download release checksums: %v\n", err)
		os.Exit(1)
	}
	expectedChecksum, err := checksumForAsset(checksumData, binaryName)
	if err != nil {
		fmt.Printf("Failed to verify release metadata: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Downloading %s...\n", binaryName)

	// Get current executable path
	execPath, err := os.Executable()
	if err != nil {
		fmt.Printf("Failed to get executable path: %v\n", err)
		os.Exit(1)
	}

	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		fmt.Printf("Failed to resolve executable path: %v\n", err)
		os.Exit(1)
	}

	// Keep the candidate on the executable's filesystem so the final rename is atomic.
	tmpFile, err := os.CreateTemp(filepath.Dir(execPath), ".etiquetta-update-*")
	if err != nil {
		fmt.Printf("Failed to create temp file: %v\n", err)
		os.Exit(1)
	}
	tmpPath := tmpFile.Name()

	actualChecksum, err := downloadBinary(downloadURL, tmpFile, maxBinarySize)
	if closeErr := tmpFile.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(tmpPath)
		fmt.Printf("Failed to download: %v\n", err)
		os.Exit(1)
	}
	if actualChecksum != expectedChecksum {
		os.Remove(tmpPath)
		fmt.Printf("Checksum mismatch for %s; refusing update\n", binaryName)
		os.Exit(1)
	}
	fmt.Println("Release checksum verified.")

	// Make executable
	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		fmt.Printf("Failed to set permissions: %v\n", err)
		os.Exit(1)
	}

	// Preserve the current executable, then atomically replace it. Never truncate it in place.
	previousPath := execPath + ".previous"
	if err := copyFileAtomic(execPath, previousPath); err != nil {
		os.Remove(tmpPath)
		fmt.Printf("Failed to preserve current binary: %v\n", err)
		os.Exit(1)
	}
	if err := os.Rename(tmpPath, execPath); err != nil {
		os.Remove(tmpPath)
		fmt.Printf("Failed to atomically install update: %v\n", err)
		os.Exit(1)
	}
	_ = syncDirectory(filepath.Dir(execPath))

	fmt.Printf("Successfully updated to %s\n", release.TagName)
	fmt.Printf("Previous binary preserved at %s\n", previousPath)

	// Try to restart the service automatically
	fmt.Println("Restarting Etiquetta...")

	// Check if systemctl is available (Linux with systemd)
	if _, err := exec.LookPath("systemctl"); err == nil {
		// Try systemd restart (may need sudo)
		cmd := exec.Command("systemctl", "restart", "etiquetta")
		if err := cmd.Run(); err != nil {
			// Try with sudo
			cmd = exec.Command("sudo", "systemctl", "restart", "etiquetta")
			if err := cmd.Run(); err != nil {
				fmt.Println("Could not restart automatically.")
				fmt.Println("Please run: sudo systemctl restart etiquetta")
				return
			}
		}
		fmt.Println("Etiquetta restarted successfully!")
		return
	}

	// Fallback for non-systemd systems
	fmt.Println("Please restart Etiquetta manually to use the new version.")
}

func downloadBytes(url string, maxBytes int64) ([]byte, error) {
	resp, err := updateHTTPClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("response exceeds %d bytes", maxBytes)
	}
	return data, nil
}

func checksumForAsset(data []byte, assetName string) (string, error) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if filepath.Base(name) != assetName {
			continue
		}
		checksum := strings.ToLower(fields[0])
		decoded, err := hex.DecodeString(checksum)
		if err != nil || len(decoded) != sha256.Size {
			return "", fmt.Errorf("invalid SHA-256 for %s", assetName)
		}
		return checksum, nil
	}
	return "", fmt.Errorf("checksum for %s not found", assetName)
}

func downloadBinary(url string, dst *os.File, maxBytes int64) (string, error) {
	resp, err := updateHTTPClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return writeVerifiedBinary(dst, resp.Body, maxBytes)
}

func writeVerifiedBinary(dst *os.File, src io.Reader, maxBytes int64) (string, error) {
	h := sha256.New()
	written, err := io.Copy(io.MultiWriter(dst, h), io.LimitReader(src, maxBytes+1))
	if err != nil {
		return "", err
	}
	if written > maxBytes {
		return "", fmt.Errorf("binary exceeds %d bytes", maxBytes)
	}
	if err := dst.Sync(); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyFileAtomic(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source is not a regular file")
	}

	tmp, err := os.CreateTemp(filepath.Dir(dstPath), ".etiquetta-previous-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, dstPath); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(dstPath))
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
