package connections

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/caioricciuti/etiquetta/internal/connections/providers"
	"github.com/caioricciuti/etiquetta/internal/database"
)

// Connection represents a stored ad platform connection
type Connection struct {
	ID              string            `json:"id"`
	Provider        string            `json:"provider"`
	Name            string            `json:"name"`
	AccountID       string            `json:"account_id"`
	Status          string            `json:"status"`
	LastSyncAt      *int64            `json:"last_sync_at"`
	LastError       *string           `json:"last_error"`
	Config          map[string]string `json:"config"`
	CreatedBy       string            `json:"created_by"`
	CreatedAt       int64             `json:"created_at"`
	UpdatedAt       int64             `json:"updated_at"`
}

// AdSpendRow represents a single row in ad_spend_daily
type AdSpendRow struct {
	ID             string `json:"id"`
	ConnectionID   string `json:"connection_id"`
	Provider       string `json:"provider"`
	Date           string `json:"date"`
	CampaignID     string `json:"campaign_id"`
	CampaignName   string `json:"campaign_name"`
	CostMicros     int64  `json:"cost_micros"`
	Impressions    int    `json:"impressions"`
	Clicks         int    `json:"clicks"`
	Currency       string `json:"currency"`
	CreatedAt      int64  `json:"created_at"`
}

// Store provides CRUD operations for connections
type Store struct {
	db        *database.DB
	secretKey string
}

// NewStore creates a new connection store
func NewStore(db *database.DB, secretKey string) *Store {
	return &Store{db: db, secretKey: secretKey}
}

// List returns all connections (without decrypted tokens)
func (s *Store) List() ([]Connection, error) {
	rows, err := s.db.Conn().Query(`
		SELECT id, provider, name, account_id, status, last_sync_at, last_error, config, created_by, created_at, updated_at
		FROM ad_connections
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list connections: %w", err)
	}
	defer rows.Close()

	var conns []Connection
	for rows.Next() {
		var c Connection
		var lastSync sql.NullInt64
		var lastErr sql.NullString
		var configJSON sql.NullString
		var accountID sql.NullString
		var createdBy sql.NullString

		if err := rows.Scan(&c.ID, &c.Provider, &c.Name, &accountID, &c.Status, &lastSync, &lastErr, &configJSON, &createdBy, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan connection: %w", err)
		}

		if lastSync.Valid {
			c.LastSyncAt = &lastSync.Int64
		}
		if lastErr.Valid {
			c.LastError = &lastErr.String
		}
		if accountID.Valid {
			c.AccountID = accountID.String
		}
		if createdBy.Valid {
			c.CreatedBy = createdBy.String
		}
		if configJSON.Valid && configJSON.String != "" {
			json.Unmarshal([]byte(configJSON.String), &c.Config)
		}
		if c.Config == nil {
			c.Config = map[string]string{}
		}

		conns = append(conns, c)
	}
	if conns == nil {
		conns = []Connection{}
	}
	return conns, nil
}

// Get returns a single connection by ID
func (s *Store) Get(id string) (*Connection, error) {
	c := &Connection{}
	var lastSync sql.NullInt64
	var lastErr sql.NullString
	var configJSON sql.NullString
	var accountID sql.NullString
	var createdBy sql.NullString

	err := s.db.Conn().QueryRow(`
		SELECT id, provider, name, account_id, status, last_sync_at, last_error, config, created_by, created_at, updated_at
		FROM ad_connections WHERE id = ?
	`, id).Scan(&c.ID, &c.Provider, &c.Name, &accountID, &c.Status, &lastSync, &lastErr, &configJSON, &createdBy, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get connection: %w", err)
	}

	if lastSync.Valid {
		c.LastSyncAt = &lastSync.Int64
	}
	if lastErr.Valid {
		c.LastError = &lastErr.String
	}
	if accountID.Valid {
		c.AccountID = accountID.String
	}
	if createdBy.Valid {
		c.CreatedBy = createdBy.String
	}
	if configJSON.Valid && configJSON.String != "" {
		json.Unmarshal([]byte(configJSON.String), &c.Config)
	}
	if c.Config == nil {
		c.Config = map[string]string{}
	}

	return c, nil
}

// Count returns the number of connections
func (s *Store) Count() (int, error) {
	var count int
	err := s.db.Conn().QueryRow("SELECT COUNT(*) FROM ad_connections").Scan(&count)
	return count, err
}

// Create inserts a new connection with encrypted tokens
func (s *Store) Create(c *Connection, tokens *providers.TokenSet) error {
	tokensJSON, err := json.Marshal(tokens)
	if err != nil {
		return fmt.Errorf("marshal tokens: %w", err)
	}

	encrypted, err := s.encrypt(tokensJSON)
	if err != nil {
		return fmt.Errorf("encrypt tokens: %w", err)
	}

	configJSON, _ := json.Marshal(c.Config)
	now := time.Now().UnixMilli()

	_, err = s.db.Conn().Exec(`
		INSERT INTO ad_connections (id, provider, name, account_id, encrypted_tokens, status, config, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.ID, c.Provider, c.Name, c.AccountID, encrypted, c.Status, string(configJSON), c.CreatedBy, now, now)
	if err != nil {
		return fmt.Errorf("insert connection: %w", err)
	}

	c.CreatedAt = now
	c.UpdatedAt = now
	return nil
}

// UpdateStatus updates a connection's status and error
func (s *Store) UpdateStatus(id, status string, lastError *string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.Conn().Exec(`
		UPDATE ad_connections SET status = ?, last_error = ?, updated_at = ? WHERE id = ?
	`, status, lastError, now, id)
	return err
}

// UpdateSyncTime records a successful sync
func (s *Store) UpdateSyncTime(id string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.Conn().Exec(`
		UPDATE ad_connections SET last_sync_at = ?, status = 'active', last_error = NULL, updated_at = ? WHERE id = ?
	`, now, now, id)
	return err
}

// Delete removes a connection and all its spend data
func (s *Store) Delete(id string) error {
	tx, err := s.db.Conn().Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	if _, err := tx.Exec("DELETE FROM ad_spend_daily WHERE connection_id = ?", id); err != nil {
		tx.Rollback()
		return fmt.Errorf("delete spend data: %w", err)
	}

	if _, err := tx.Exec("DELETE FROM ad_connections WHERE id = ?", id); err != nil {
		tx.Rollback()
		return fmt.Errorf("delete connection: %w", err)
	}

	return tx.Commit()
}

// GetTokens decrypts and returns the stored tokens for a connection
func (s *Store) GetTokens(id string) (*providers.TokenSet, error) {
	var encrypted string
	err := s.db.Conn().QueryRow("SELECT encrypted_tokens FROM ad_connections WHERE id = ?", id).Scan(&encrypted)
	if err != nil {
		return nil, fmt.Errorf("get tokens: %w", err)
	}

	decrypted, err := s.decrypt(encrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypt tokens: %w", err)
	}

	var tokens providers.TokenSet
	if err := json.Unmarshal(decrypted, &tokens); err != nil {
		return nil, fmt.Errorf("unmarshal tokens: %w", err)
	}

	return &tokens, nil
}

// UpdateTokens re-encrypts and stores updated tokens
func (s *Store) UpdateTokens(id string, tokens *providers.TokenSet) error {
	tokensJSON, err := json.Marshal(tokens)
	if err != nil {
		return fmt.Errorf("marshal tokens: %w", err)
	}

	encrypted, err := s.encrypt(tokensJSON)
	if err != nil {
		return fmt.Errorf("encrypt tokens: %w", err)
	}

	now := time.Now().UnixMilli()
	_, err = s.db.Conn().Exec(`
		UPDATE ad_connections SET encrypted_tokens = ?, updated_at = ? WHERE id = ?
	`, encrypted, now, id)
	return err
}

// InsertSpendData bulk inserts daily spend rows
func (s *Store) InsertSpendData(rows []AdSpendRow) error {
	if len(rows) == 0 {
		return nil
	}

	tx, err := s.db.Conn().Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO ad_spend_daily (id, connection_id, provider, date, campaign_id, campaign_name, cost_micros, impressions, clicks, currency, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			cost_micros = EXCLUDED.cost_micros,
			impressions = EXCLUDED.impressions,
			clicks = EXCLUDED.clicks,
			campaign_name = EXCLUDED.campaign_name
	`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("prepare stmt: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UnixMilli()
	for _, r := range rows {
		_, err := stmt.Exec(r.ID, r.ConnectionID, r.Provider, r.Date, r.CampaignID, r.CampaignName, r.CostMicros, r.Impressions, r.Clicks, r.Currency, now)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("insert spend row: %w", err)
		}
	}

	return tx.Commit()
}

// --- AES-256-GCM encryption helpers ---

func (s *Store) deriveKey() []byte {
	h := sha256.Sum256([]byte(s.secretKey + ":connections"))
	return h[:]
}

func (s *Store) encrypt(plaintext []byte) (string, error) {
	key := s.deriveKey()
	block, err := aes.NewCipher(key)
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

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (s *Store) decrypt(encoded string) ([]byte, error) {
	key := s.deriveKey()
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}
