package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/caioricciuti/etiquetta/internal/auth"
	"github.com/caioricciuti/etiquetta/internal/config"
	"github.com/caioricciuti/etiquetta/internal/database"
	"github.com/caioricciuti/etiquetta/internal/enrichment"
	"github.com/go-chi/chi/v5"
)

func newConsentSecurityFixture(t *testing.T) (*Handlers, *database.DB, http.Handler) {
	t.Helper()

	dataDir := t.TempDir()
	db, err := database.New(filepath.Join(dataDir, "consent-test.duckdb"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	cfg := &config.Config{SecretKey: "consent-security-test-secret"}
	h := &Handlers{
		db:       db,
		enricher: enrichment.New(filepath.Join(dataDir, "missing.mmdb")),
		cfg:      cfg,
		auth:     auth.New(cfg.SecretKey, false),
	}

	router := chi.NewRouter()
	router.Get("/consent/{siteId}/config", h.GetPublicConsentConfig)
	router.Post("/consent/{siteId}/record", h.RecordConsent)
	router.Get("/api/auth/setup", h.CheckSetup)
	router.Post("/api/auth/setup", h.Setup)
	return h, db, router
}

func seedConsentSecurityConfig(t *testing.T, db *database.DB) {
	t.Helper()
	if _, err := db.Conn().Exec(`
		INSERT INTO domains (id, name, domain, site_id, created_at, is_active)
		VALUES ('domain-1', 'Example', 'example.test', 'site-1', 1, 1)
	`); err != nil {
		t.Fatalf("insert domain: %v", err)
	}
	if _, err := db.Conn().Exec(`
		INSERT INTO consent_configs (
			id, domain_id, version, is_active, categories, appearance,
			translations, cookie_name, cookie_expiry_days, auto_language,
			geo_targeting, created_at, updated_at
		) VALUES (
			'config-1', 'domain-1', 3, 1,
			'[{"id":"necessary","required":true},{"id":"analytics","required":false},{"id":"marketing","required":false}]',
			'{}', '{}', 'etiquetta_consent', 365, 1, '[]', 1, 1
		)
	`); err != nil {
		t.Fatalf("insert consent config: %v", err)
	}
}

func performConsentRecordRequest(router http.Handler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/consent/site-1/record", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "consent-test")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

func TestPublicConsentConfigIsNotCacheable(t *testing.T) {
	_, db, router := newConsentSecurityFixture(t)
	seedConsentSecurityConfig(t, db)

	request := httptest.NewRequest(http.MethodGet, "/consent/site-1/config", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}

func TestPublicConsentConfigDoesNotTreatDatabaseFailureAsDisabled(t *testing.T) {
	_, db, router := newConsentSecurityFixture(t)
	if err := db.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/consent/site-1/config", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusServiceUnavailable, response.Body.String())
	}
}

func TestRecordConsentValidatesActiveConfigAndDecisionSemantics(t *testing.T) {
	_, db, router := newConsentSecurityFixture(t)
	seedConsentSecurityConfig(t, db)

	valid := `{"visitor_hash":"visitor-1","categories":{"necessary":true,"analytics":false,"marketing":true},"config_version":3,"action":"custom"}`
	if response := performConsentRecordRequest(router, valid); response.Code != http.StatusNoContent {
		t.Fatalf("valid status = %d, want %d; body = %s", response.Code, http.StatusNoContent, response.Body.String())
	}

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "unknown action",
			body:       `{"visitor_hash":"visitor-1","categories":{},"config_version":3,"action":"grant"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "stale config version",
			body:       `{"visitor_hash":"visitor-1","categories":{},"config_version":2,"action":"show"}`,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "required category disabled",
			body:       `{"visitor_hash":"visitor-1","categories":{"necessary":false,"analytics":true,"marketing":true},"config_version":3,"action":"custom"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "unknown category",
			body:       `{"visitor_hash":"visitor-1","categories":{"necessary":true,"analytics":false,"other":false},"config_version":3,"action":"custom"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "reject all grants optional category",
			body:       `{"visitor_hash":"visitor-1","categories":{"necessary":true,"analytics":true,"marketing":false},"config_version":3,"action":"reject_all"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "trailing JSON",
			body:       `{"visitor_hash":"visitor-1","categories":{},"config_version":3,"action":"show"}{}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performConsentRecordRequest(router, test.body)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, test.wantStatus, response.Body.String())
			}
		})
	}

	var count int
	if err := db.Conn().QueryRow("SELECT COUNT(*) FROM consent_records").Scan(&count); err != nil {
		t.Fatalf("count consent records: %v", err)
	}
	if count != 1 {
		t.Fatalf("consent record count = %d, want 1", count)
	}
}

func TestInitialSetupHasExactlyOneConcurrentWinner(t *testing.T) {
	_, db, router := newConsentSecurityFixture(t)

	bodies := []string{
		`{"email":"first@example.test","password":"correct-horse-1","name":"First"}`,
		`{"email":"second@example.test","password":"correct-horse-2","name":"Second"}`,
	}
	responses := make([]*httptest.ResponseRecorder, len(bodies))
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range bodies {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			req := httptest.NewRequest(http.MethodPost, "/api/auth/setup", strings.NewReader(bodies[index]))
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			responses[index] = response
		}(i)
	}
	close(start)
	wg.Wait()

	statusCounts := map[int]int{}
	for _, response := range responses {
		statusCounts[response.Code]++
	}
	if statusCounts[http.StatusOK] != 1 || statusCounts[http.StatusBadRequest] != 1 {
		t.Fatalf("setup statuses = %#v, want one 200 and one 400", statusCounts)
	}

	var adminCount int
	if err := db.Conn().QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&adminCount); err != nil {
		t.Fatalf("count admins: %v", err)
	}
	if adminCount != 1 {
		t.Fatalf("admin count = %d, want 1", adminCount)
	}

	var setupComplete string
	if err := db.Conn().QueryRow("SELECT value FROM settings WHERE key = 'setup_complete'").Scan(&setupComplete); err != nil {
		t.Fatalf("read setup marker: %v", err)
	}
	if setupComplete != "true" {
		t.Fatalf("setup_complete = %q, want true", setupComplete)
	}
}

func TestBrowserRuntimesUseFailClosedConsentContract(t *testing.T) {
	tracker, err := trackerJS.ReadFile("tracker.js")
	if err != nil {
		t.Fatalf("read tracker: %v", err)
	}
	trackerSource := string(tracker)
	for _, required := range []string{
		`var VISITOR_HASH = null;`,
		`if (!consent) return false;`,
		`return consent.analytics !== false;`,
		`if (status === "loading" || status === "error") return false;`,
		`window.__ETIQUETTA_CONSENT_STATUS__ = "loading";`,
		`if (!analyticsConsentAllowsTracking()) return;`,
	} {
		if !strings.Contains(trackerSource, required) {
			t.Errorf("tracker is missing consent contract fragment %q", required)
		}
	}
	if strings.Contains(trackerSource, `var VISITOR_HASH = getOrCreateVisitorId();`) {
		t.Error("tracker creates a visitor identifier before consent resolves")
	}

	consent, err := consentJS.ReadFile("consent.js")
	if err != nil {
		t.Fatalf("read consent manager: %v", err)
	}
	consentSource := string(consent)
	for _, required := range []string{
		`window.__ETIQUETTA_CONSENT_STATUS__ = 'loading';`,
		`finish(null, 'unconfigured');`,
		`finish(null, 'error');`,
		`setConsentState(defaultConsent(config.categories || []));`,
		`'etiquetta:consent-ready'`,
	} {
		if !strings.Contains(consentSource, required) {
			t.Errorf("consent manager is missing readiness fragment %q", required)
		}
	}

	snapshot, err := json.Marshal(map[string]interface{}{
		"tags":      []interface{}{},
		"triggers":  []interface{}{},
		"variables": []interface{}{},
	})
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	containerSource := generateContainerJS(string(snapshot), false)
	for _, required := range []string{
		`if(cat==="necessary")return true;`,
		`return status==="unconfigured";`,
		`if(status==="loading")return;`,
		`window.addEventListener("etiquetta:consent-ready",initForConsentState);`,
	} {
		if !strings.Contains(containerSource, required) {
			t.Errorf("container is missing consent contract fragment %q", required)
		}
	}
	if strings.Contains(containerSource, `return !consent||(consent[cat]===true);`) {
		t.Error("container still treats missing consent state as approval")
	}
}
