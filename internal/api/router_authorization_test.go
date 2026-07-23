package api

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/caioricciuti/etiquetta/internal/auth"
	"github.com/caioricciuti/etiquetta/internal/buffer"
	"github.com/caioricciuti/etiquetta/internal/config"
	"github.com/caioricciuti/etiquetta/internal/connections"
	"github.com/caioricciuti/etiquetta/internal/database"
	"github.com/caioricciuti/etiquetta/internal/enrichment"
	"github.com/caioricciuti/etiquetta/internal/licensing"
	"github.com/caioricciuti/etiquetta/internal/migrate"
	"github.com/caioricciuti/etiquetta/internal/replay"
)

const authorizationTestSecret = "router-authorization-test-secret"

func newAuthorizationTestRouter(t *testing.T) http.Handler {
	t.Helper()
	router, _ := newAuthorizationTestRouterWithDB(t)
	return router
}

func newAuthorizationTestRouterWithDB(t *testing.T) (http.Handler, *database.DB) {
	t.Helper()

	dataDir := t.TempDir()
	dbPath := filepath.Join(dataDir, "test.duckdb")
	db, err := database.New(dbPath)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	if _, err := db.Conn().Exec(`
		INSERT INTO domains (id, name, domain, site_id, created_by, created_at, is_active)
		VALUES ('domain-id', 'Assigned domain', 'assigned.example', 'site_assigned', 'test-admin', ?, 1)
	`, time.Now().UnixMilli()); err != nil {
		t.Fatalf("seed assigned domain: %v", err)
	}
	if _, err := db.Conn().Exec(`
		INSERT INTO domain_access (user_id, domain_id, created_at)
		VALUES ('test-viewer', 'domain-id', ?)
	`, time.Now().UnixMilli()); err != nil {
		t.Fatalf("seed viewer domain access: %v", err)
	}

	bufferManager := buffer.NewBufferManager(db.Conn(), buffer.BufferConfig{
		Threshold:    100,
		FlushTimeout: time.Hour,
		TempDir:      filepath.Join(dataDir, "buffer_tmp"),
		CompactHour:  3,
		DuckDBPath:   dbPath,
	})
	t.Cleanup(func() { bufferManager.Close(context.Background()) })

	cfg := &config.Config{
		DataDir:               dataDir,
		GeoIPPath:             filepath.Join(dataDir, "missing.mmdb"),
		SessionTimeoutMinutes: 30,
		AllowedOrigins:        []string{"http://example.test"},
		SecretKey:             authorizationTestSecret,
	}
	licenseManager := licensing.NewManager(filepath.Join(dataDir, "license.json"))
	connectionStore := connections.NewStore(db, cfg.SecretKey)
	syncManager := connections.NewSyncManager(connectionStore, time.Hour)
	migrateManager := migrate.NewJobManager(migrate.NewStore(db.Conn()), bufferManager, dataDir)
	replayManager := replay.NewStore(dataDir)
	uiFS := fs.FS(fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<!doctype html><title>test</title>")},
	})

	router := NewRouter(
		db,
		enrichment.New(cfg.GeoIPPath),
		licenseManager,
		cfg,
		uiFS,
		bufferManager,
		connectionStore,
		syncManager,
		migrateManager,
		replayManager,
	)
	return router, db
}

func authorizationTestToken(t *testing.T, role string) string {
	return authorizationTestTokenForUser(t, "test-"+role, role)
}

func authorizationTestTokenForUser(t *testing.T, userID, role string) string {
	t.Helper()
	token, err := auth.New(authorizationTestSecret, false).GenerateToken(&auth.User{
		ID:    userID,
		Email: role + "@example.test",
		Role:  role,
	})
	if err != nil {
		t.Fatalf("generate %s token: %v", role, err)
	}
	return token
}

func seedUnassignedAuthorizationDomain(t *testing.T, db *database.DB) {
	t.Helper()
	if _, err := db.Conn().Exec(`
		INSERT INTO domains (id, name, domain, site_id, created_by, created_at, is_active)
		VALUES ('other-domain-id', 'Other domain', 'other.example', 'site_other', 'test-admin', ?, 1)
	`, time.Now().UnixMilli()); err != nil {
		t.Fatalf("seed unassigned domain: %v", err)
	}
}

func seedAuthorizationEvents(t *testing.T, db *database.DB) {
	t.Helper()
	now := time.Now().UnixMilli()
	if _, err := db.Conn().Exec(`
		INSERT INTO events
			(id, timestamp, event_type, session_id, visitor_hash, domain, url, path, is_bot)
		VALUES
			('assigned-event', ?, 'pageview', 'assigned-session', 'assigned-visitor', 'assigned.example', 'https://assigned.example/', '/', 0),
			('other-event', ?, 'pageview', 'other-session', 'other-visitor', 'other.example', 'https://other.example/private', '/private', 0)
	`, now, now); err != nil {
		t.Fatalf("seed domain events: %v", err)
	}
}

func performAuthorizationRequest(router http.Handler, token, method, path string) *httptest.ResponseRecorder {
	return performAuthorizationRequestWithBody(router, token, method, path, "")
}

func performAuthorizationRequestWithBody(router http.Handler, token, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

func TestSensitiveRoutesRejectViewerBeforeHandlerOrLicenseCheck(t *testing.T) {
	router := newAuthorizationTestRouter(t)
	viewerToken := authorizationTestToken(t, "viewer")

	tests := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/license"},
		{http.MethodDelete, "/api/license"},
		{http.MethodGet, "/api/settings"},
		{http.MethodPut, "/api/settings"},
		{http.MethodGet, "/api/db"},
		{http.MethodGet, "/api/db/info"},
		{http.MethodPost, "/api/tokens"},
		{http.MethodDelete, "/api/tokens/token-id"},
		{http.MethodPost, "/api/domains"},
		{http.MethodDelete, "/api/domains/domain-id"},
		{http.MethodPost, "/api/share-links"},
		{http.MethodDelete, "/api/share-links/share-id"},
		{http.MethodPost, "/api/annotations"},
		{http.MethodPut, "/api/annotations/annotation-id"},
		{http.MethodDelete, "/api/annotations/annotation-id"},
		{http.MethodPost, "/api/funnels"},
		{http.MethodPut, "/api/funnels/funnel-id"},
		{http.MethodDelete, "/api/funnels/funnel-id"},
		{http.MethodPost, "/api/goals"},
		{http.MethodPut, "/api/goals/goal-id"},
		{http.MethodDelete, "/api/goals/goal-id"},
		{http.MethodPost, "/api/cohorts"},
		{http.MethodPut, "/api/cohorts/cohort-id"},
		{http.MethodDelete, "/api/cohorts/cohort-id"},
		{http.MethodPost, "/api/alerts"},
		{http.MethodPut, "/api/alerts/alert-id"},
		{http.MethodDelete, "/api/alerts/alert-id"},
		{http.MethodPost, "/api/webhooks"},
		{http.MethodPut, "/api/webhooks/webhook-id"},
		{http.MethodDelete, "/api/webhooks/webhook-id"},
		{http.MethodPost, "/api/webhooks/webhook-id/test"},
		{http.MethodGet, "/api/campaigns"},
		{http.MethodGet, "/api/campaigns/campaign-id/report"},
		{http.MethodPost, "/api/campaigns"},
		{http.MethodDelete, "/api/campaigns/campaign-id"},
		{http.MethodPost, "/api/consent/configs/domain-id"},
		{http.MethodPut, "/api/consent/configs/domain-id/toggle"},
		{http.MethodPost, "/api/tagmanager/containers"},
		{http.MethodPut, "/api/tagmanager/containers/container-id"},
		{http.MethodDelete, "/api/tagmanager/containers/container-id"},
		{http.MethodPost, "/api/tagmanager/containers/container-id/publish"},
		{http.MethodPost, "/api/tagmanager/containers/container-id/rollback/1"},
		{http.MethodPost, "/api/tagmanager/containers/container-id/import"},
		{http.MethodPost, "/api/tagmanager/containers/container-id/preview-token"},
		{http.MethodGet, "/api/tagmanager/pick-result?token=preview-token"},
		{http.MethodPost, "/api/tagmanager/containers/container-id/tags"},
		{http.MethodPut, "/api/tagmanager/containers/container-id/tags/tag-id"},
		{http.MethodDelete, "/api/tagmanager/containers/container-id/tags/tag-id"},
		{http.MethodPost, "/api/tagmanager/containers/container-id/triggers"},
		{http.MethodPut, "/api/tagmanager/containers/container-id/triggers/trigger-id"},
		{http.MethodDelete, "/api/tagmanager/containers/container-id/triggers/trigger-id"},
		{http.MethodPost, "/api/tagmanager/containers/container-id/variables"},
		{http.MethodPut, "/api/tagmanager/containers/container-id/variables/variable-id"},
		{http.MethodDelete, "/api/tagmanager/containers/container-id/variables/variable-id"},
		{http.MethodPost, "/api/connections"},
		{http.MethodPut, "/api/connections/connection-id/tokens"},
		{http.MethodDelete, "/api/connections/connection-id"},
		{http.MethodPost, "/api/connections/connection-id/sync"},
		{http.MethodPut, "/api/replays/settings"},
		{http.MethodDelete, "/api/replays/batch"},
		{http.MethodDelete, "/api/replays/session-id"},
	}

	for _, test := range tests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			response := performAuthorizationRequest(router, viewerToken, test.method, test.path)
			if response.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusForbidden, response.Body.String())
			}
		})
	}
}

func TestViewerReadRoutesRemainOutsideAdminMiddleware(t *testing.T) {
	router := newAuthorizationTestRouter(t)
	viewerToken := authorizationTestToken(t, "viewer")

	tests := []struct {
		path       string
		wantStatus int
	}{
		// Community read: reaches the domain handler successfully.
		{"/api/domains", http.StatusOK},
		{"/api/tokens", http.StatusOK},
		{"/api/share-links?domain_id=domain-id", http.StatusOK},
		{"/api/annotations?domain_id=domain-id", http.StatusOK},
		// Licensed-feature reads reach the license middleware. A missing test
		// license returns 402; a misplaced admin middleware would return 403.
		{"/api/funnels", http.StatusPaymentRequired},
		{"/api/goals", http.StatusPaymentRequired},
		{"/api/cohorts", http.StatusPaymentRequired},
		{"/api/alerts", http.StatusPaymentRequired},
		{"/api/webhooks", http.StatusPaymentRequired},
		{"/api/consent/configs/domain-id", http.StatusPaymentRequired},
		{"/api/tagmanager/containers", http.StatusPaymentRequired},
		{"/api/connections", http.StatusOK},
		{"/api/replays", http.StatusPaymentRequired},
		{"/api/replays/settings", http.StatusPaymentRequired},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			response := performAuthorizationRequest(router, viewerToken, http.MethodGet, test.path)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, test.wantStatus, response.Body.String())
			}
		})
	}
}

func TestViewerDomainIsolationRejectsBlankAndUnassignedReads(t *testing.T) {
	router, db := newAuthorizationTestRouterWithDB(t)
	seedUnassignedAuthorizationDomain(t, db)
	seedAuthorizationEvents(t, db)
	now := time.Now().UnixMilli()
	if _, err := db.Conn().Exec(`
		INSERT INTO ad_connections
			(id, provider, name, encrypted_tokens, status, config, created_at, updated_at, domain_id)
		VALUES
			('assigned-connection', 'google_ads', 'Assigned connection', 'unused', 'active', '{}', ?, ?, 'domain-id'),
			('other-connection', 'google_ads', 'Other connection', 'unused', 'active', '{}', ?, ?, 'other-domain-id')
	`, now, now, now, now); err != nil {
		t.Fatalf("seed connections: %v", err)
	}
	viewerToken := authorizationTestToken(t, "viewer")

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{name: "assigned hostname", path: "/api/stats/overview?domain=assigned.example", wantStatus: http.StatusOK},
		{name: "blank aggregate", path: "/api/stats/overview", wantStatus: http.StatusForbidden},
		{name: "unassigned hostname", path: "/api/stats/overview?domain=other.example", wantStatus: http.StatusForbidden},
		{name: "assigned domain id", path: "/api/share-links?domain_id=domain-id", wantStatus: http.StatusOK},
		{name: "blank domain id", path: "/api/share-links", wantStatus: http.StatusForbidden},
		{name: "unassigned domain id", path: "/api/share-links?domain_id=other-domain-id", wantStatus: http.StatusForbidden},
		{name: "assigned path domain", path: "/api/domains/domain-id/snippet", wantStatus: http.StatusOK},
		{name: "unassigned path domain", path: "/api/domains/other-domain-id/snippet", wantStatus: http.StatusForbidden},
		{name: "assigned connection", path: "/api/connections/assigned-connection", wantStatus: http.StatusOK},
		{name: "unassigned connection", path: "/api/connections/other-connection", wantStatus: http.StatusForbidden},
		{name: "unassigned connection filter", path: "/api/connections?domain_id=other-domain-id", wantStatus: http.StatusForbidden},
		{name: "blank ad spend aggregate", path: "/api/stats/ad-spend", wantStatus: http.StatusForbidden},
		{name: "assigned ad spend", path: "/api/stats/ad-spend?domain=assigned.example", wantStatus: http.StatusOK},
		{name: "unassigned ad spend", path: "/api/stats/ad-spend?domain=other.example", wantStatus: http.StatusForbidden},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performAuthorizationRequest(router, viewerToken, http.MethodGet, test.path)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, test.wantStatus, response.Body.String())
			}
		})
	}

	noAccessToken := authorizationTestTokenForUser(t, "viewer-without-access", "viewer")
	response := performAuthorizationRequest(router, noAccessToken, http.MethodGet, "/api/stats/overview?domain=assigned.example")
	if response.Code != http.StatusForbidden {
		t.Fatalf("viewer without assignments status = %d, want %d; body = %s", response.Code, http.StatusForbidden, response.Body.String())
	}

	connectionsResponse := performAuthorizationRequest(router, viewerToken, http.MethodGet, "/api/connections")
	if connectionsResponse.Code != http.StatusOK {
		t.Fatalf("viewer connections status = %d; body = %s", connectionsResponse.Code, connectionsResponse.Body.String())
	}
	var connectionList []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(connectionsResponse.Body.Bytes(), &connectionList); err != nil {
		t.Fatalf("decode viewer connections: %v", err)
	}
	if len(connectionList) != 1 || connectionList[0].ID != "assigned-connection" {
		t.Fatalf("viewer connections = %#v, want only assigned connection", connectionList)
	}

	statsResponse := performAuthorizationRequest(router, viewerToken, http.MethodGet, "/api/stats/overview?domain=assigned.example")
	var viewerStats map[string]any
	if err := json.Unmarshal(statsResponse.Body.Bytes(), &viewerStats); err != nil {
		t.Fatalf("decode viewer stats: %v", err)
	}
	if viewerStats["total_events"] != float64(1) {
		t.Fatalf("viewer total_events = %#v, want only assigned event", viewerStats["total_events"])
	}
}

func TestViewerDomainListAndAdminReadsRespectRoleScope(t *testing.T) {
	router, db := newAuthorizationTestRouterWithDB(t)
	seedUnassignedAuthorizationDomain(t, db)
	seedAuthorizationEvents(t, db)

	viewerResponse := performAuthorizationRequest(router, authorizationTestToken(t, "viewer"), http.MethodGet, "/api/domains")
	if viewerResponse.Code != http.StatusOK {
		t.Fatalf("viewer domains status = %d; body = %s", viewerResponse.Code, viewerResponse.Body.String())
	}
	var domains []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(viewerResponse.Body.Bytes(), &domains); err != nil {
		t.Fatalf("decode viewer domains: %v", err)
	}
	if len(domains) != 1 || domains[0].ID != "domain-id" {
		t.Fatalf("viewer domains = %#v, want only assigned domain", domains)
	}

	adminToken := authorizationTestToken(t, "admin")
	for _, path := range []string{
		"/api/stats/overview",
		"/api/stats/overview?domain=other.example",
		"/api/domains/other-domain-id/snippet",
	} {
		response := performAuthorizationRequest(router, adminToken, http.MethodGet, path)
		if response.Code != http.StatusOK {
			t.Errorf("admin GET %s status = %d, want %d; body = %s", path, response.Code, http.StatusOK, response.Body.String())
		}
	}

	adminStatsResponse := performAuthorizationRequest(router, adminToken, http.MethodGet, "/api/stats/overview")
	var adminStats map[string]any
	if err := json.Unmarshal(adminStatsResponse.Body.Bytes(), &adminStats); err != nil {
		t.Fatalf("decode admin stats: %v", err)
	}
	if adminStats["total_events"] != float64(2) {
		t.Fatalf("admin total_events = %#v, want aggregate of both domains", adminStats["total_events"])
	}
}

func TestRealtimeNotificationsAreFilteredByViewerDomainScope(t *testing.T) {
	viewerClient := make(chan []byte, 1)
	adminClient := make(chan []byte, 1)
	noAccessClient := make(chan []byte, 1)
	h := &Handlers{sseClients: map[chan []byte]domainAccessScope{
		viewerClient: {
			domainNames: map[string]struct{}{"assigned.example": {}},
		},
		adminClient: {unrestricted: true},
		noAccessClient: {
			domainNames: map[string]struct{}{},
		},
	}}

	h.notifyClients([]*database.Event{
		{Domain: "assigned.example", EventType: "pageview", Path: "/assigned"},
		{Domain: "other.example", EventType: "pageview", Path: "/private"},
	}, nil, nil)

	decodeNotification := func(t *testing.T, client <-chan []byte) map[string]any {
		t.Helper()
		select {
		case data := <-client:
			var result map[string]any
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatalf("decode notification: %v", err)
			}
			return result
		default:
			t.Fatal("expected notification")
			return nil
		}
	}

	viewerNotification := decodeNotification(t, viewerClient)
	if viewerNotification["events"] != float64(1) {
		t.Fatalf("viewer event count = %#v, want 1", viewerNotification["events"])
	}
	lastEvent, _ := viewerNotification["last_event"].(map[string]any)
	if lastEvent["path"] != "/assigned" {
		t.Fatalf("viewer last event = %#v, want assigned event", lastEvent)
	}

	adminNotification := decodeNotification(t, adminClient)
	if adminNotification["events"] != float64(2) {
		t.Fatalf("admin event count = %#v, want 2", adminNotification["events"])
	}

	select {
	case data := <-noAccessClient:
		t.Fatalf("viewer without access received notification: %s", data)
	default:
	}
}

func TestViewerSelfServiceRoutesRemainAvailable(t *testing.T) {
	router := newAuthorizationTestRouter(t)
	viewerToken := authorizationTestToken(t, "viewer")

	tests := []struct {
		method     string
		path       string
		wantStatus int
	}{
		// Empty payloads are rejected by the handlers. A misplaced admin
		// middleware would instead return 403 before reaching them.
		{http.MethodPut, "/api/auth/profile", http.StatusBadRequest},
		{http.MethodPost, "/api/auth/password", http.StatusBadRequest},
		{http.MethodPost, "/api/auth/logout", http.StatusNoContent},
	}

	for _, test := range tests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			response := performAuthorizationRequest(router, viewerToken, test.method, test.path)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, test.wantStatus, response.Body.String())
			}
		})
	}
}

func TestLicenseDetailsRequireAuthenticationAndAreSanitizedForViewer(t *testing.T) {
	router := newAuthorizationTestRouter(t)

	publicResponse := httptest.NewRecorder()
	router.ServeHTTP(publicResponse, httptest.NewRequest(http.MethodGet, "/api/license", nil))
	if publicResponse.Code != http.StatusUnauthorized {
		t.Fatalf("public status = %d, want %d; body = %s", publicResponse.Code, http.StatusUnauthorized, publicResponse.Body.String())
	}

	viewerResponse := performAuthorizationRequest(router, authorizationTestToken(t, "viewer"), http.MethodGet, "/api/license")
	if viewerResponse.Code != http.StatusOK {
		t.Fatalf("viewer status = %d, want %d; body = %s", viewerResponse.Code, http.StatusOK, viewerResponse.Body.String())
	}
	var viewerInfo map[string]any
	if err := json.Unmarshal(viewerResponse.Body.Bytes(), &viewerInfo); err != nil {
		t.Fatalf("decode viewer license: %v", err)
	}
	for _, key := range []string{"licensee", "expires_at"} {
		if _, exists := viewerInfo[key]; exists {
			t.Errorf("viewer response exposed %q", key)
		}
	}
	if _, exists := viewerInfo["features"]; !exists {
		t.Error("viewer response omitted feature capabilities")
	}

	adminResponse := performAuthorizationRequest(router, authorizationTestToken(t, "admin"), http.MethodGet, "/api/license")
	if adminResponse.Code != http.StatusOK {
		t.Fatalf("admin status = %d, want %d; body = %s", adminResponse.Code, http.StatusOK, adminResponse.Body.String())
	}
	var adminInfo map[string]any
	if err := json.Unmarshal(adminResponse.Body.Bytes(), &adminInfo); err != nil {
		t.Fatalf("decode admin license: %v", err)
	}
	for _, key := range []string{"licensee", "expires_at"} {
		if _, exists := adminInfo[key]; !exists {
			t.Errorf("admin response omitted %q", key)
		}
	}
}

func TestSettingsResponseOmitsRestrictedKeys(t *testing.T) {
	router := newAuthorizationTestRouter(t)
	adminToken := authorizationTestToken(t, "admin")

	response := performAuthorizationRequest(router, adminToken, http.MethodGet, "/api/settings")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}

	var result map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode settings response: %v", err)
	}
	for _, key := range []string{"secret_key", "maxmind_account_id", "maxmind_license_key", "setup_complete"} {
		if _, exists := result[key]; exists {
			t.Errorf("restricted key %q was serialized", key)
		}
	}
	if result["respect_dnt"] != "true" {
		t.Errorf("non-sensitive setting missing from response: respect_dnt = %q", result["respect_dnt"])
	}
}

func TestUpdateSettingsRejectsRestrictedBatchBeforeWriting(t *testing.T) {
	router := newAuthorizationTestRouter(t)
	adminToken := authorizationTestToken(t, "admin")

	for _, key := range []string{"secret_key", "setup_complete", "smtp_password", "future_service_api_token"} {
		t.Run(key, func(t *testing.T) {
			body := `{"respect_dnt":"false","` + key + `":"attacker-controlled"}`
			response := performAuthorizationRequestWithBody(router, adminToken, http.MethodPut, "/api/settings", body)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusBadRequest, response.Body.String())
			}

			settingsResponse := performAuthorizationRequest(router, adminToken, http.MethodGet, "/api/settings")
			if settingsResponse.Code != http.StatusOK {
				t.Fatalf("get settings status = %d; body = %s", settingsResponse.Code, settingsResponse.Body.String())
			}
			var result map[string]string
			if err := json.Unmarshal(settingsResponse.Body.Bytes(), &result); err != nil {
				t.Fatalf("decode settings response: %v", err)
			}
			if result["respect_dnt"] != "true" {
				t.Fatalf("safe key was partially written before %q rejection", key)
			}
		})
	}
}
