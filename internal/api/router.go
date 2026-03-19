package api

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/caioricciuti/etiquetta/internal/auth"
	"github.com/caioricciuti/etiquetta/internal/buffer"
	"github.com/caioricciuti/etiquetta/internal/config"
	"github.com/caioricciuti/etiquetta/internal/connections"
	"github.com/caioricciuti/etiquetta/internal/database"
	"github.com/caioricciuti/etiquetta/internal/enrichment"
	"github.com/caioricciuti/etiquetta/internal/identification"
	"github.com/caioricciuti/etiquetta/internal/licensing"
	"github.com/caioricciuti/etiquetta/internal/migrate"
	"github.com/caioricciuti/etiquetta/internal/replay"
)

//go:embed tracker.js
var trackerJS embed.FS

//go:embed consent.js
var consentJS embed.FS

//go:embed recorder.js
var recorderJS embed.FS

//go:embed rrweb.min.js
var rrwebJS embed.FS

// NewRouter creates the HTTP router
func NewRouter(db *database.DB, enricher *enrichment.Enricher, licenseManager *licensing.Manager, cfg *config.Config, uiFS fs.FS, bufferMgr *buffer.BufferManager, connStore *connections.Store, syncManager *connections.SyncManager, migrateManager *migrate.JobManager, replayMgr *replay.Store) http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(RequestLogger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)
	r.Use(middleware.Compress(5))

	// CORS - allow credentials for auth cookies
	// Use AllowOriginFunc instead of AllowedOrigins to reflect the actual
	// Origin header. AllowedOrigins: ["*"] sends a literal "*" which browsers
	// reject when credentials are included (sendBeacon, fetch with cookies).
	r.Use(cors.Handler(cors.Options{
		AllowOriginFunc: func(r *http.Request, origin string) bool {
			for _, o := range cfg.AllowedOrigins {
				if o == "*" || o == origin {
					return true
				}
			}
			return false
		},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Content-Type", "X-Requested-With", "Authorization"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Create auth service
	// secureCookie should only be true when running with HTTPS directly
	// When behind a reverse proxy (nginx), the proxy handles HTTPS
	// Check ETIQUETTA_SECURE_COOKIES env var, default to false for proxy setups
	secureCookie := os.Getenv("ETIQUETTA_SECURE_COOKIES") == "true"
	authService := auth.New(cfg.SecretKey, secureCookie)
	authMiddleware := auth.NewMiddleware(authService)

	// Create identity generator
	idGen := identification.New(cfg.SecretKey, cfg.SessionTimeoutMinutes)

	// Create handlers
	h := &Handlers{
		db:             db,
		enricher:       enricher,
		licenseManager: licenseManager,
		idGen:          idGen,
		cfg:            cfg,
		auth:           authService,
		bufferMgr:      bufferMgr,
		connStore:      connStore,
		syncManager:    syncManager,
		migrateManager: migrateManager,
	}

	// Wire up API key validation so Bearer etq_... tokens work through RequireAuth
	authMiddleware.SetAPIKeyValidator(h.ValidateAPIKey)

	// Set the replay store for handlers
	replayStore = replayMgr

	// ========== Public endpoints ==========

	// Tracker script - serve at /s.js (clean URL)
	r.Get("/s.js", h.ServeTrackerScript)
	r.Get("/s/tracker.js", h.ServeTrackerScript) // Legacy URL

	// Ingest endpoint (rate limited: 100 req/min/IP)
	r.With(RateLimit(100, time.Minute)).Post("/i", h.Ingest)

	// Session replay ingest (rate limited: 30 req/min/IP — larger payloads)
	r.With(RateLimit(30, time.Minute)).Post("/r", h.IngestReplay)

	// Replay config for tracker (public)
	r.Get("/r/config", h.ServeReplayConfig)

	// Replay recorder script (public)
	r.Get("/r.js", h.ServeRecorderScript)

	// Self-hosted rrweb library (public, immutable cache)
	r.Get("/r/rrweb.min.js", h.ServeRrwebScript)

	// Consent banner script
	r.Get("/c.js", h.ServeConsentScript)

	// Consent public endpoints
	r.Get("/consent/{siteId}/config", h.GetPublicConsentConfig)
	r.With(RateLimit(60, time.Minute)).Post("/consent/{siteId}/record", h.RecordConsent)

	// Tag Manager container script
	r.Get("/tm/{siteId}.js", h.ServeContainerScript)

	// robots.txt (dynamic, based on AI crawler settings)
	r.Get("/robots.txt", h.ServeRobotsTxt)

	// Health check
	r.Get("/health", h.Health)

	// Version endpoint (public)
	r.Get("/api/version", h.GetVersion)

	// ========== API routes ==========
	r.Route("/api", func(r chi.Router) {

		// Auth routes (public)
		r.Route("/auth", func(r chi.Router) {
			r.Get("/setup", h.CheckSetup)
			r.Post("/setup", h.Setup)
			r.Post("/login", h.Login)
			r.Post("/logout", h.Logout)

			// Protected auth routes
			r.Group(func(r chi.Router) {
				r.Use(authMiddleware.RequireAuth)
				r.Get("/me", h.GetCurrentUser)
				r.Put("/profile", h.UpdateProfile)
				r.Post("/password", h.ChangePassword)
			})
		})

		// License info (public - needed for UI to check features)
		r.Get("/license", h.GetLicense)

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(authMiddleware.RequireAuth)

			// License management
			r.Post("/license", h.UploadLicense)
			r.Delete("/license", h.RemoveLicense)

			// Settings
			r.Get("/settings", h.GetSettings)
			r.Put("/settings", h.UpdateSettings)

			// GeoIP Settings (admin only)
			r.Group(func(r chi.Router) {
				r.Use(authMiddleware.RequireAdmin)
				r.Get("/settings/geoip", h.GetGeoIPSettings)
				r.Put("/settings/geoip", h.UpdateGeoIPSettings)
				r.Get("/settings/geoip/status", h.GetGeoIPStatus)
				r.Post("/settings/geoip/download", h.DownloadGeoIPDatabase)
			})

			// AI Crawler Settings (admin only)
			r.Group(func(r chi.Router) {
				r.Use(authMiddleware.RequireAdmin)
				r.Get("/settings/ai-crawlers", h.GetAICrawlerSettings)
				r.Put("/settings/ai-crawlers", h.UpdateAICrawlerSettings)
			})

			// Email Settings (admin only)
			r.Group(func(r chi.Router) {
				r.Use(authMiddleware.RequireAdmin)
				r.Get("/settings/email", h.GetEmailSettings)
				r.Put("/settings/email", h.UpdateEmailSettings)
				r.Post("/settings/email/test", h.TestEmailSettings)
			})

			// Database access
			r.Get("/db", h.ServeDatabase)
			r.Get("/db/info", h.GetDatabaseInfo)

			// Real-time events via SSE
			r.Get("/events/stream", h.EventStream)

			// Stats endpoints
			r.Get("/stats/overview", h.GetStatsOverview)
			r.Get("/stats/timeseries", h.GetStatsTimeseries)
			r.Get("/stats/pages", h.GetStatsPages)
			r.Get("/stats/referrers", h.GetStatsReferrers)
			r.Get("/stats/geo", h.GetStatsGeo)
			r.Get("/stats/map", h.GetStatsMapData)
			r.Get("/stats/devices", h.GetStatsDevices)
			r.Get("/stats/browsers", h.GetStatsBrowsers)
			r.Get("/stats/campaigns", h.GetStatsCampaigns)
			r.Get("/stats/events", h.GetStatsCustomEvents)
			r.Get("/stats/events/summary", h.GetStatsEventsSummary)
			r.Get("/stats/events/timeseries", h.GetStatsEventsTimeseries)
			r.Get("/stats/events/props", h.GetStatsEventsProps)
			r.Get("/stats/outbound", h.GetStatsOutbound)
			r.Get("/stats/bots", h.GetStatsBots)                       // Bot traffic breakdown
			r.Get("/stats/calendar-heatmap", h.GetStatsCalendarHeatmap) // Calendar heatmap data
			r.Get("/stats/compare", h.GetStatsCompare)                   // Period comparison

			// API token management
			r.Post("/tokens", h.CreateAPIToken)
			r.Get("/tokens", h.ListAPITokens)
			r.Delete("/tokens/{id}", h.RevokeAPIToken)

			// Domain management
			r.Get("/domains", h.ListDomains)
			r.Post("/domains", h.CreateDomain)
			r.Delete("/domains/{id}", h.DeleteDomain)
			r.Get("/domains/{id}/snippet", h.GetDomainSnippet)

			// Pro features - Web Vitals
			r.Group(func(r chi.Router) {
				r.Use(licensing.RequireFeature(licenseManager, licensing.FeaturePerformance))
				r.Get("/stats/vitals", h.GetStatsVitals)
			})

			// Pro features - Error tracking
			r.Group(func(r chi.Router) {
				r.Use(licensing.RequireFeature(licenseManager, licensing.FeatureErrorTracking))
				r.Get("/stats/errors", h.GetStatsErrors)
			})

			// Pro features - Export
			r.Group(func(r chi.Router) {
				r.Use(licensing.RequireFeature(licenseManager, licensing.FeatureExport))
				r.Get("/export/events", h.ExportEvents)
			})

			// Pro features - Ad Fraud Detection
			r.Group(func(r chi.Router) {
				r.Use(licensing.RequireFeature(licenseManager, licensing.FeatureAdFraud))
				r.Get("/stats/fraud", h.GetFraudSummary)
				r.Get("/sources/quality", h.GetSourceQuality)
				r.Get("/campaigns", h.ListCampaigns)
				r.Post("/campaigns", h.CreateCampaign)
				r.Get("/campaigns/{id}/report", h.GetCampaignReport)
				r.Delete("/campaigns/{id}", h.DeleteCampaign)
			})

			// Pro features - Consent Management
			r.Group(func(r chi.Router) {
				r.Use(licensing.RequireFeature(licenseManager, licensing.FeatureConsent))
				r.Get("/consent/configs/{domainId}", h.GetConsentConfig)
				r.Post("/consent/configs/{domainId}", h.SaveConsentConfig)
				r.Put("/consent/configs/{domainId}/toggle", h.ToggleConsentBanner)
				r.Get("/consent/configs/{domainId}/history", h.GetConsentConfigHistory)
				r.Get("/consent/analytics/{domainId}", h.GetConsentAnalytics)
				r.Get("/consent/records/{domainId}", h.GetConsentRecords)
			})

			// Pro features - Tag Manager
			r.Group(func(r chi.Router) {
				r.Use(licensing.RequireFeature(licenseManager, licensing.FeatureTagManager))
				r.Get("/tagmanager/containers", h.ListContainers)
				r.Post("/tagmanager/containers", h.CreateContainer)
				r.Get("/tagmanager/containers/{id}", h.GetContainer)
				r.Put("/tagmanager/containers/{id}", h.UpdateContainer)
				r.Delete("/tagmanager/containers/{id}", h.DeleteContainer)
				r.Post("/tagmanager/containers/{id}/publish", h.PublishContainer)
				r.Get("/tagmanager/containers/{id}/versions", h.GetContainerVersions)
				r.Post("/tagmanager/containers/{id}/rollback/{version}", h.RollbackContainer)
				r.Get("/tagmanager/containers/{id}/export", h.ExportContainer)
				r.Post("/tagmanager/containers/{id}/import", h.ImportContainer)
				r.Post("/tagmanager/containers/{id}/preview-token", h.PreviewToken)

				// Tag CRUD
				r.Get("/tagmanager/containers/{cid}/tags", h.ListTags)
				r.Post("/tagmanager/containers/{cid}/tags", h.CreateTag)
				r.Get("/tagmanager/containers/{cid}/tags/{id}", h.GetTag)
				r.Put("/tagmanager/containers/{cid}/tags/{id}", h.UpdateTag)
				r.Delete("/tagmanager/containers/{cid}/tags/{id}", h.DeleteTag)

				// Trigger CRUD
				r.Get("/tagmanager/containers/{cid}/triggers", h.ListTriggers)
				r.Post("/tagmanager/containers/{cid}/triggers", h.CreateTrigger)
				r.Put("/tagmanager/containers/{cid}/triggers/{id}", h.UpdateTrigger)
				r.Delete("/tagmanager/containers/{cid}/triggers/{id}", h.DeleteTrigger)

				// Variable CRUD
				r.Get("/tagmanager/containers/{cid}/variables", h.ListVariables)
				r.Post("/tagmanager/containers/{cid}/variables", h.CreateVariable)
				r.Put("/tagmanager/containers/{cid}/variables/{id}", h.UpdateVariable)
				r.Delete("/tagmanager/containers/{cid}/variables/{id}", h.DeleteVariable)
			})

			// Connections (ad platform integrations)
			r.Group(func(r chi.Router) {
				r.Use(licensing.RequireFeature(licenseManager, licensing.FeatureConnections))
				r.Get("/connections", h.ListConnections)
				r.Post("/connections", h.CreateConnection)
				r.Get("/connections/providers", h.GetProviders)
				r.Get("/connections/{id}", h.GetConnection)
				r.Put("/connections/{id}/tokens", h.UpdateConnectionToken)
				r.Delete("/connections/{id}", h.DeleteConnection)
				r.Post("/connections/{id}/sync", h.SyncConnection)
				r.Get("/stats/ad-spend", h.GetAdSpend)
				r.Get("/stats/ad-attribution", h.GetAdAttribution)
			})

			// Google Ads Settings (admin only)
			r.Group(func(r chi.Router) {
				r.Use(authMiddleware.RequireAdmin)
				r.Get("/settings/google-ads", h.GetGoogleAdsSettings)
				r.Put("/settings/google-ads", h.UpdateGoogleAdsSettings)
				r.Post("/settings/google-ads/test", h.TestGoogleAdsSettings)
			})

			// Meta Ads Settings (admin only)
			r.Group(func(r chi.Router) {
				r.Use(authMiddleware.RequireAdmin)
				r.Get("/settings/meta-ads", h.GetMetaAdsSettings)
				r.Put("/settings/meta-ads", h.UpdateMetaAdsSettings)
				r.Post("/settings/meta-ads/test", h.TestMetaAdsSettings)
			})

			// Microsoft Ads Settings (admin only)
			r.Group(func(r chi.Router) {
				r.Use(authMiddleware.RequireAdmin)
				r.Get("/settings/microsoft-ads", h.GetMicrosoftAdsSettings)
				r.Put("/settings/microsoft-ads", h.UpdateMicrosoftAdsSettings)
				r.Post("/settings/microsoft-ads/test", h.TestMicrosoftAdsSettings)
			})

			// Admin only - User management
			r.Group(func(r chi.Router) {
				r.Use(authMiddleware.RequireAdmin)
				r.Use(licensing.RequireFeature(licenseManager, licensing.FeatureMultiUser))
				r.Get("/users", h.ListUsers)
				r.Post("/users", h.CreateUser)
				r.Put("/users/{id}", h.UpdateUser)
				r.Delete("/users/{id}", h.DeleteUser)
			})

			// Admin only - Privacy / GDPR
			r.Group(func(r chi.Router) {
				r.Use(authMiddleware.RequireAdmin)
				r.Get("/privacy/audit", h.GetPrivacyAudit)
				r.Get("/privacy/audit-log", h.GetAuditLog)
				r.Get("/privacy/export/{visitorHash}", h.ExportVisitorData)
				r.Get("/privacy/erasure/{visitorHash}", h.LookupVisitorData)
				r.Delete("/privacy/erasure/{visitorHash}", h.EraseVisitorData)
			})

			// Admin only - Data Explorer
			r.Group(func(r chi.Router) {
				r.Use(authMiddleware.RequireAdmin)
				r.Post("/explorer/query", h.ExplorerQuery)
				r.Get("/explorer/schema", h.ExplorerSchema)
			})

			// Migration tools (admin only, all tiers)
			r.Group(func(r chi.Router) {
				r.Use(authMiddleware.RequireAdmin)
				r.Post("/migrate/analyze", h.MigrateAnalyze)
				r.Post("/migrate/start", h.MigrateStart)
				r.Get("/migrate/jobs", h.MigrateListJobs)
				r.Get("/migrate/jobs/{id}", h.MigrateGetJob)
				r.Delete("/migrate/jobs/{id}", h.MigrateRollback)
				r.Post("/migrate/jobs/{id}/cancel", h.MigrateCancelJob)
				r.Post("/migrate/gtm/convert", h.MigrateGTMConvert)
			})

			// Pro features - Session Replay
			r.Group(func(r chi.Router) {
				r.Use(licensing.RequireFeature(licenseManager, licensing.FeatureSessionReplay))
				r.Get("/replays", h.ListReplays)
				r.Get("/replays/stats", h.GetReplayStats)
				r.Get("/replays/settings", h.GetReplaySettings)
				r.Put("/replays/settings", h.UpdateReplaySettings)
				r.Get("/replays/{sessionId}", h.GetReplay)
				r.Get("/replays/{sessionId}/events", h.GetSessionEvents)
				r.Delete("/replays/{sessionId}", h.DeleteReplay)
			})
		})
	})

	// Element Picker proxy (token-validated, no auth middleware needed)
	r.Get("/api/tagmanager/pick-proxy", h.PickProxy)
	r.With(RateLimit(200, time.Minute)).Get("/_etq_proxy/{token}/{scheme}/{host}/*", h.PickProxyResource)

	// Serve static UI files from embedded filesystem
	fileServer := http.FileServer(http.FS(uiFS))
	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		path := req.URL.Path

		// Try to serve the file directly
		if path != "/" {
			// Check if file exists
			filePath := strings.TrimPrefix(path, "/")
			if f, err := uiFS.Open(filePath); err == nil {
				f.Close()
				fileServer.ServeHTTP(w, req)
				return
			}
		}

		// Serve index.html for SPA routes
		indexFile, err := uiFS.Open("index.html")
		if err != nil {
			http.NotFound(w, req)
			return
		}
		defer indexFile.Close()

		stat, _ := indexFile.Stat()
		content, _ := fs.ReadFile(uiFS, "index.html")
		http.ServeContent(w, req, "index.html", stat.ModTime(), strings.NewReader(string(content)))
	})

	return r
}

