package settings

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Sensitive keys that should be encrypted
var sensitiveKeys = map[string]bool{
	"secret_key":                true,
	"maxmind_account_id":        true,
	"maxmind_license_key":       true,
	"smtp_password":             true,
	"resend_api_key":            true,
	"google_ads_client_id":      true,
	"google_ads_client_secret":  true,
	"google_ads_developer_token":    true,
	"meta_ads_app_id":               true,
	"meta_ads_app_secret":           true,
	"microsoft_ads_client_id":       true,
	"microsoft_ads_client_secret":   true,
	"microsoft_ads_developer_token": true,
}

// Service manages application settings stored in the database
type Service struct {
	db        *sql.DB
	cache     map[string]string
	cacheMu   sync.RWMutex
	masterKey []byte
}

// New creates a new settings service
func New(db *sql.DB) *Service {
	s := &Service{
		db:    db,
		cache: make(map[string]string),
	}
	return s
}

// SetMasterKey sets the encryption key for sensitive settings
func (s *Service) SetMasterKey(key string) {
	hash := sha256.Sum256([]byte(key))
	s.masterKey = hash[:]
}

// Get retrieves a setting value
func (s *Service) Get(key string) (string, error) {
	s.cacheMu.RLock()
	if val, ok := s.cache[key]; ok {
		s.cacheMu.RUnlock()
		return val, nil
	}
	s.cacheMu.RUnlock()

	var value string
	err := s.db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}

	// Decrypt if sensitive
	if sensitiveKeys[key] && value != "" && s.masterKey != nil {
		decrypted, err := s.decrypt(value)
		if err == nil {
			value = decrypted
		}
		// If decryption fails, return the value as-is (might be unencrypted)
	}

	s.cacheMu.Lock()
	s.cache[key] = value
	s.cacheMu.Unlock()

	return value, nil
}

// GetWithDefault retrieves a setting value with a default fallback
func (s *Service) GetWithDefault(key, defaultValue string) string {
	val, err := s.Get(key)
	if err != nil || val == "" {
		return defaultValue
	}
	return val
}

// GetInt retrieves a setting as an integer
func (s *Service) GetInt(key string, defaultValue int) int {
	val, err := s.Get(key)
	if err != nil || val == "" {
		return defaultValue
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return defaultValue
	}
	return i
}

// GetBool retrieves a setting as a boolean
func (s *Service) GetBool(key string, defaultValue bool) bool {
	val, err := s.Get(key)
	if err != nil || val == "" {
		return defaultValue
	}
	return val == "true" || val == "1" || val == "yes"
}

// Set stores a setting value
func (s *Service) Set(key, value string) error {
	// Encrypt if sensitive
	storedValue := value
	if sensitiveKeys[key] && value != "" && s.masterKey != nil {
		encrypted, err := s.encrypt(value)
		if err != nil {
			return err
		}
		storedValue = encrypted
	}

	now := time.Now().UnixMilli()
	_, err := s.db.Exec(
		"INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at",
		key, storedValue, now,
	)
	if err != nil {
		return err
	}

	// Update cache with decrypted value
	s.cacheMu.Lock()
	s.cache[key] = value
	s.cacheMu.Unlock()

	return nil
}

// SetMany stores multiple settings at once
func (s *Service) SetMany(settings map[string]string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UnixMilli()
	stmt, err := tx.Prepare("INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for key, value := range settings {
		storedValue := value
		if sensitiveKeys[key] && value != "" && s.masterKey != nil {
			encrypted, err := s.encrypt(value)
			if err != nil {
				return err
			}
			storedValue = encrypted
		}

		_, err := stmt.Exec(key, storedValue, now)
		if err != nil {
			return err
		}

		// Update cache with decrypted value
		s.cacheMu.Lock()
		s.cache[key] = value
		s.cacheMu.Unlock()
	}

	return tx.Commit()
}

// GetAll retrieves all settings
func (s *Service) GetAll() (map[string]string, error) {
	rows, err := s.db.Query("SELECT key, value FROM settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}

		// Decrypt if sensitive
		if sensitiveKeys[key] && value != "" && s.masterKey != nil {
			decrypted, err := s.decrypt(value)
			if err == nil {
				value = decrypted
			}
		}

		settings[key] = value
	}

	return settings, nil
}

// GetAllMasked returns all settings with sensitive values masked
func (s *Service) GetAllMasked() (map[string]string, error) {
	settings, err := s.GetAll()
	if err != nil {
		return nil, err
	}

	for key := range settings {
		if sensitiveKeys[key] && settings[key] != "" {
			settings[key] = maskValue(settings[key])
		}
	}

	return settings, nil
}

// Delete removes a setting
func (s *Service) Delete(key string) error {
	_, err := s.db.Exec("DELETE FROM settings WHERE key = ?", key)
	if err != nil {
		return err
	}

	s.cacheMu.Lock()
	delete(s.cache, key)
	s.cacheMu.Unlock()

	return nil
}

// ClearCache clears the settings cache
func (s *Service) ClearCache() {
	s.cacheMu.Lock()
	s.cache = make(map[string]string)
	s.cacheMu.Unlock()
}

// GenerateSecretKey generates a new random secret key
func GenerateSecretKey() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// encrypt encrypts a value using AES-GCM
func (s *Service) encrypt(plaintext string) (string, error) {
	if s.masterKey == nil {
		return plaintext, nil
	}

	block, err := aes.NewCipher(s.masterKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return "enc:" + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decrypt decrypts a value using AES-GCM
func (s *Service) decrypt(ciphertext string) (string, error) {
	if s.masterKey == nil {
		return ciphertext, nil
	}

	// Check if value is encrypted (has enc: prefix)
	if !strings.HasPrefix(ciphertext, "enc:") {
		return ciphertext, nil
	}

	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(ciphertext, "enc:"))
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(s.masterKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// maskValue masks a sensitive value for display
func maskValue(value string) string {
	if len(value) <= 4 {
		return "****"
	}
	return value[:2] + strings.Repeat("*", len(value)-4) + value[len(value)-2:]
}

// IsSensitive checks if a key is sensitive
func IsSensitive(key string) bool {
	return sensitiveKeys[key]
}
