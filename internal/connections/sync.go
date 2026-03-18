package connections

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/caioricciuti/etiquetta/internal/connections/providers"
)

// SyncManager handles periodic syncing of ad spend data from connected providers
type SyncManager struct {
	store    *Store
	interval time.Duration
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewSyncManager creates a new sync manager
func NewSyncManager(store *Store, interval time.Duration) *SyncManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &SyncManager{
		store:    store,
		interval: interval,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start begins the periodic sync loop
func (sm *SyncManager) Start() {
	sm.wg.Add(1)
	go func() {
		defer sm.wg.Done()
		log.Printf("[connections] sync manager started (interval: %s)", sm.interval)

		// Run immediately on start
		sm.syncAll()

		ticker := time.NewTicker(sm.interval)
		defer ticker.Stop()

		for {
			select {
			case <-sm.ctx.Done():
				log.Println("[connections] sync manager stopped")
				return
			case <-ticker.C:
				sm.syncAll()
			}
		}
	}()
}

// Stop gracefully shuts down the sync manager
func (sm *SyncManager) Stop() {
	sm.cancel()
	sm.wg.Wait()
}

// SyncConnection triggers a sync for a single connection
func (sm *SyncManager) SyncConnection(connID string) error {
	conn, err := sm.store.Get(connID)
	if err != nil {
		return fmt.Errorf("get connection: %w", err)
	}

	return sm.syncOne(conn)
}

func (sm *SyncManager) syncAll() {
	conns, err := sm.store.List()
	if err != nil {
		log.Printf("[connections] failed to list connections: %v", err)
		return
	}

	for _, conn := range conns {
		if conn.Status == "disconnected" {
			continue
		}

		if err := sm.syncOne(&conn); err != nil {
			log.Printf("[connections] sync failed for %s (%s): %v", conn.Name, conn.Provider, err)
			errStr := err.Error()
			sm.store.UpdateStatus(conn.ID, "error", &errStr)
		}
	}
}

func (sm *SyncManager) syncOne(conn *Connection) error {
	provider, ok := providers.Get(conn.Provider)
	if !ok {
		return fmt.Errorf("unknown provider: %s", conn.Provider)
	}

	tokens, err := sm.store.GetTokens(conn.ID)
	if err != nil {
		return fmt.Errorf("get tokens: %w", err)
	}

	// Refresh token if expired
	if time.Now().After(tokens.ExpiresAt) {
		newTokens, err := provider.RefreshToken(sm.ctx, tokens.RefreshToken)
		if err != nil {
			return fmt.Errorf("refresh token: %w", err)
		}
		if err := sm.store.UpdateTokens(conn.ID, newTokens); err != nil {
			return fmt.Errorf("update tokens: %w", err)
		}
		tokens = newTokens
	}

	// Fetch last 7 days of data (or since last sync)
	end := time.Now().Truncate(24 * time.Hour)
	start := end.AddDate(0, 0, -7)
	if conn.LastSyncAt != nil {
		syncTime := time.UnixMilli(*conn.LastSyncAt)
		// Go back one extra day to catch any late-arriving data
		if syncTime.AddDate(0, 0, -1).After(start) {
			start = syncTime.AddDate(0, 0, -1)
		}
	}

	spendData, err := provider.FetchDailySpend(sm.ctx, tokens.AccessToken, conn.AccountID, start, end)
	if err != nil {
		return fmt.Errorf("fetch spend: %w", err)
	}

	// Convert to rows
	rows := make([]AdSpendRow, 0, len(spendData))
	for _, d := range spendData {
		rows = append(rows, AdSpendRow{
			ID:           generateSyncID(conn.ID, d.Date, d.CampaignID),
			ConnectionID: conn.ID,
			Provider:     conn.Provider,
			Date:         d.Date,
			CampaignID:   d.CampaignID,
			CampaignName: d.CampaignName,
			CostMicros:   d.CostMicros,
			Impressions:  d.Impressions,
			Clicks:       d.Clicks,
			Currency:     d.Currency,
		})
	}

	if err := sm.store.InsertSpendData(rows); err != nil {
		return fmt.Errorf("insert spend data: %w", err)
	}

	sm.store.UpdateSyncTime(conn.ID)
	log.Printf("[connections] synced %d rows for %s (%s)", len(rows), conn.Name, conn.Provider)
	return nil
}

// generateSyncID creates a deterministic ID for upsert deduplication
func generateSyncID(connID, date, campaignID string) string {
	h := sha256.Sum256([]byte(connID + "|" + date + "|" + campaignID))
	return hex.EncodeToString(h[:16])
}
