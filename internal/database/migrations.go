package database

import (
	"fmt"
)

// Migrate runs database migrations
func (db *DB) Migrate() error {
	// Create migrations table
	_, err := db.conn.Exec(`
		CREATE TABLE IF NOT EXISTS migrations (
			version INTEGER PRIMARY KEY,
			applied_at BIGINT NOT NULL
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Get current version
	var currentVersion int
	row := db.conn.QueryRow("SELECT COALESCE(MAX(version), 0) FROM migrations")
	row.Scan(&currentVersion)

	// Run migrations
	migrations := []struct {
		version int
		sql     string
	}{
		{
			version: 1,
			sql: `
				-- Events table (pageviews, custom events, clicks, scroll, identify)
				CREATE TABLE IF NOT EXISTS events (
					id VARCHAR PRIMARY KEY,
					timestamp BIGINT NOT NULL,
					event_type VARCHAR NOT NULL,
					event_name VARCHAR,
					session_id VARCHAR NOT NULL,
					visitor_hash VARCHAR NOT NULL,
					user_id VARCHAR,
					domain VARCHAR NOT NULL,
					url VARCHAR NOT NULL,
					path VARCHAR NOT NULL,
					page_title VARCHAR,
					referrer_url VARCHAR,
					referrer_type VARCHAR,
					utm_source VARCHAR,
					utm_medium VARCHAR,
					utm_campaign VARCHAR,
					geo_country VARCHAR,
					geo_city VARCHAR,
					geo_region VARCHAR,
					browser_name VARCHAR,
					os_name VARCHAR,
					device_type VARCHAR,
					is_bot INTEGER DEFAULT 0,
					props VARCHAR DEFAULT '{}'
				);

				-- Indexes for events
				CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);
				CREATE INDEX IF NOT EXISTS idx_events_session ON events(session_id);
				CREATE INDEX IF NOT EXISTS idx_events_visitor ON events(visitor_hash);
				CREATE INDEX IF NOT EXISTS idx_events_domain ON events(domain);
				CREATE INDEX IF NOT EXISTS idx_events_path ON events(path);
				CREATE INDEX IF NOT EXISTS idx_events_type ON events(event_type);
				CREATE INDEX IF NOT EXISTS idx_events_country ON events(geo_country);
			`,
		},
		{
			version: 2,
			sql: `
				-- Performance table (Core Web Vitals)
				CREATE TABLE IF NOT EXISTS performance (
					id VARCHAR PRIMARY KEY,
					timestamp BIGINT NOT NULL,
					session_id VARCHAR NOT NULL,
					visitor_hash VARCHAR NOT NULL,
					domain VARCHAR NOT NULL,
					url VARCHAR NOT NULL,
					path VARCHAR NOT NULL,
					lcp DOUBLE,
					cls DOUBLE,
					fcp DOUBLE,
					ttfb DOUBLE,
					inp DOUBLE,
					page_load_time DOUBLE,
					device_type VARCHAR,
					connection_type VARCHAR,
					geo_country VARCHAR
				);

				-- Indexes for performance
				CREATE INDEX IF NOT EXISTS idx_perf_timestamp ON performance(timestamp);
				CREATE INDEX IF NOT EXISTS idx_perf_session ON performance(session_id);
				CREATE INDEX IF NOT EXISTS idx_perf_path ON performance(path);
			`,
		},
		{
			version: 3,
			sql: `
				-- Errors table (JS errors, resource failures)
				CREATE TABLE IF NOT EXISTS errors (
					id VARCHAR PRIMARY KEY,
					timestamp BIGINT NOT NULL,
					session_id VARCHAR NOT NULL,
					visitor_hash VARCHAR NOT NULL,
					domain VARCHAR NOT NULL,
					url VARCHAR NOT NULL,
					path VARCHAR NOT NULL,
					error_type VARCHAR NOT NULL,
					error_message VARCHAR NOT NULL,
					error_stack VARCHAR,
					error_hash VARCHAR NOT NULL,
					script_url VARCHAR,
					line_number INTEGER,
					column_number INTEGER,
					browser_name VARCHAR,
					geo_country VARCHAR
				);

				-- Indexes for errors
				CREATE INDEX IF NOT EXISTS idx_errors_timestamp ON errors(timestamp);
				CREATE INDEX IF NOT EXISTS idx_errors_hash ON errors(error_hash);
				CREATE INDEX IF NOT EXISTS idx_errors_type ON errors(error_type);
				CREATE INDEX IF NOT EXISTS idx_errors_session ON errors(session_id);
			`,
		},
		{
			version: 4,
			sql: `
				-- Settings table
				CREATE TABLE IF NOT EXISTS settings (
					key VARCHAR PRIMARY KEY,
					value VARCHAR NOT NULL,
					updated_at BIGINT NOT NULL
				);

				-- Insert default settings
				INSERT INTO settings (key, value, updated_at)
				VALUES
					('track_performance', 'true', epoch_ms(now())),
					('track_errors', 'true', epoch_ms(now())),
					('session_timeout_minutes', '30', epoch_ms(now())),
					('respect_dnt', 'true', epoch_ms(now()))
				ON CONFLICT (key) DO NOTHING;
			`,
		},
		{
			version: 5,
			sql: `
				-- Users table (for auth)
				CREATE TABLE IF NOT EXISTS users (
					id VARCHAR PRIMARY KEY,
					email VARCHAR UNIQUE NOT NULL,
					password_hash VARCHAR NOT NULL,
					name VARCHAR,
					role VARCHAR DEFAULT 'viewer',
					created_at BIGINT NOT NULL,
					updated_at BIGINT NOT NULL
				);

				-- Sessions table (for auth)
				CREATE TABLE IF NOT EXISTS sessions (
					id VARCHAR PRIMARY KEY,
					user_id VARCHAR NOT NULL,
					expires_at BIGINT NOT NULL,
					created_at BIGINT NOT NULL
				);

				CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
				CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);
			`,
		},
		{
			version: 6,
			sql: `
				-- Domains table (for multi-site tracking)
				CREATE TABLE IF NOT EXISTS domains (
					id VARCHAR PRIMARY KEY,
					name VARCHAR NOT NULL,
					domain VARCHAR UNIQUE NOT NULL,
					created_by VARCHAR,
					created_at BIGINT NOT NULL,
					is_active INTEGER DEFAULT 1
				);

				CREATE INDEX IF NOT EXISTS idx_domains_domain ON domains(domain);
				CREATE INDEX IF NOT EXISTS idx_domains_active ON domains(is_active);

				-- Add setup_complete setting
				INSERT INTO settings (key, value, updated_at)
				VALUES ('setup_complete', 'false', epoch_ms(now()))
				ON CONFLICT (key) DO NOTHING;
			`,
		},
		{
			version: 7,
			sql: `
				-- Add site_id to domains for tracking script authentication
				ALTER TABLE domains ADD COLUMN site_id VARCHAR;

				-- Generate site_id for existing domains
				UPDATE domains SET site_id = 'site_' || substring(md5(random()::VARCHAR), 1, 16) WHERE site_id IS NULL;

				-- Create unique index on site_id
				CREATE UNIQUE INDEX IF NOT EXISTS idx_domains_site_id ON domains(site_id);
			`,
		},
		{
			version: 8,
			sql: `
				-- Bot detection columns
				ALTER TABLE events ADD COLUMN bot_score INTEGER DEFAULT 0;
				ALTER TABLE events ADD COLUMN bot_signals VARCHAR DEFAULT '[]';
				ALTER TABLE events ADD COLUMN bot_category VARCHAR DEFAULT 'human';

				-- Behavioral flags
				ALTER TABLE events ADD COLUMN has_scroll INTEGER DEFAULT 0;
				ALTER TABLE events ADD COLUMN has_mouse_move INTEGER DEFAULT 0;
				ALTER TABLE events ADD COLUMN has_click INTEGER DEFAULT 0;
				ALTER TABLE events ADD COLUMN has_touch INTEGER DEFAULT 0;

				-- Click tracking for fraud detection
				ALTER TABLE events ADD COLUMN click_x INTEGER;
				ALTER TABLE events ADD COLUMN click_y INTEGER;
				ALTER TABLE events ADD COLUMN page_duration INTEGER;

				-- IP classification
				ALTER TABLE events ADD COLUMN datacenter_ip INTEGER DEFAULT 0;
				ALTER TABLE events ADD COLUMN ip_hash VARCHAR;

				-- Indexes for bot filtering
				CREATE INDEX IF NOT EXISTS idx_events_bot_category ON events(bot_category);
				CREATE INDEX IF NOT EXISTS idx_events_bot_score ON events(bot_score);
			`,
		},
		{
			version: 9,
			sql: `
				-- Campaigns table for ad fraud detection
				CREATE TABLE IF NOT EXISTS campaigns (
					id VARCHAR PRIMARY KEY,
					name VARCHAR NOT NULL,
					utm_source VARCHAR,
					utm_medium VARCHAR,
					utm_campaign VARCHAR,
					cpc DOUBLE DEFAULT 0,
					cpm DOUBLE DEFAULT 0,
					budget DOUBLE DEFAULT 0,
					start_date BIGINT,
					end_date BIGINT,
					created_at BIGINT NOT NULL
				);

				-- Visitor sessions table (materialized)
				CREATE TABLE IF NOT EXISTS visitor_sessions (
					id VARCHAR PRIMARY KEY,
					session_id VARCHAR UNIQUE,
					visitor_hash VARCHAR,
					domain VARCHAR,
					start_time BIGINT,
					end_time BIGINT,
					duration BIGINT,
					pageviews INTEGER,
					entry_url VARCHAR,
					exit_url VARCHAR,
					is_bounce INTEGER,
					bot_score INTEGER,
					bot_category VARCHAR
				);

				-- Indexes for campaigns and sessions
				CREATE INDEX IF NOT EXISTS idx_campaigns_utm ON campaigns(utm_source, utm_medium, utm_campaign);
				CREATE INDEX IF NOT EXISTS idx_visitor_sessions_session ON visitor_sessions(session_id);
				CREATE INDEX IF NOT EXISTS idx_visitor_sessions_domain ON visitor_sessions(domain);
				CREATE INDEX IF NOT EXISTS idx_visitor_sessions_bot ON visitor_sessions(bot_category);
			`,
		},
		{
			version: 10,
			sql: `
				-- Add geo coordinates for map plotting
				ALTER TABLE events ADD COLUMN geo_latitude DOUBLE;
				ALTER TABLE events ADD COLUMN geo_longitude DOUBLE;

				-- Index for geo queries
				CREATE INDEX IF NOT EXISTS idx_events_geo ON events(geo_latitude, geo_longitude);
			`,
		},
		{
			version: 11,
			sql: `
				-- Add settings for configuration
				INSERT INTO settings (key, value, updated_at) VALUES
					('secret_key', '', epoch_ms(now())),
					('maxmind_account_id', '', epoch_ms(now())),
					('maxmind_license_key', '', epoch_ms(now())),
					('geoip_path', './data/GeoLite2-City.mmdb', epoch_ms(now())),
					('geoip_auto_update', 'false', epoch_ms(now())),
					('geoip_last_updated', '', epoch_ms(now())),
					('allowed_origins', '*', epoch_ms(now())),
					('listen_addr', ':3456', epoch_ms(now()))
				ON CONFLICT (key) DO NOTHING;
			`,
		},
		{
			version: 12,
			sql: `
				-- Composite index for the most common stat query pattern:
				-- WHERE timestamp >= ? AND timestamp <= ? AND is_bot = 0 AND domain = ?
				CREATE INDEX IF NOT EXISTS idx_events_ts_domain_bot
					ON events(timestamp, domain, is_bot);

				-- Composite index for pageview stats (adds event_type)
				CREATE INDEX IF NOT EXISTS idx_events_ts_type_bot
					ON events(timestamp, event_type, is_bot);

				-- Composite for visitor_sessions queries
				CREATE INDEX IF NOT EXISTS idx_vsessions_domain_start
					ON visitor_sessions(domain, start_time);
			`,
		},
		{
			version: 13,
			sql: `
				-- Consent tables
				CREATE TABLE IF NOT EXISTS consent_configs (
					id VARCHAR PRIMARY KEY,
					domain_id VARCHAR NOT NULL,
					version INTEGER NOT NULL DEFAULT 1,
					is_active INTEGER NOT NULL DEFAULT 1,
					categories VARCHAR NOT NULL DEFAULT '[]',
					appearance VARCHAR NOT NULL DEFAULT '{}',
					translations VARCHAR NOT NULL DEFAULT '{}',
					cookie_name VARCHAR NOT NULL DEFAULT 'etiquetta_consent',
					cookie_expiry_days INTEGER NOT NULL DEFAULT 365,
					auto_language INTEGER NOT NULL DEFAULT 1,
					geo_targeting VARCHAR NOT NULL DEFAULT '[]',
					created_at BIGINT NOT NULL,
					updated_at BIGINT NOT NULL
				);
				CREATE INDEX IF NOT EXISTS idx_consent_configs_domain ON consent_configs(domain_id);
				CREATE UNIQUE INDEX IF NOT EXISTS idx_consent_configs_version ON consent_configs(domain_id, version);

				CREATE TABLE IF NOT EXISTS consent_records (
					id VARCHAR PRIMARY KEY,
					domain_id VARCHAR NOT NULL,
					visitor_hash VARCHAR NOT NULL,
					ip_hash VARCHAR,
					categories VARCHAR NOT NULL DEFAULT '{}',
					config_version INTEGER NOT NULL,
					action VARCHAR NOT NULL,
					user_agent VARCHAR,
					geo_country VARCHAR,
					timestamp BIGINT NOT NULL
				);
				CREATE INDEX IF NOT EXISTS idx_consent_records_domain_ts ON consent_records(domain_id, timestamp);
				CREATE INDEX IF NOT EXISTS idx_consent_records_visitor ON consent_records(visitor_hash);

				-- Tag Manager tables
				CREATE TABLE IF NOT EXISTS tm_containers (
					id VARCHAR PRIMARY KEY,
					domain_id VARCHAR NOT NULL,
					name VARCHAR NOT NULL,
					published_version INTEGER DEFAULT 0,
					draft_version INTEGER DEFAULT 1,
					published_at BIGINT,
					published_by VARCHAR,
					created_at BIGINT NOT NULL,
					updated_at BIGINT NOT NULL,
					UNIQUE(domain_id)
				);

				CREATE TABLE IF NOT EXISTS tm_tags (
					id VARCHAR PRIMARY KEY,
					container_id VARCHAR NOT NULL,
					name VARCHAR NOT NULL,
					tag_type VARCHAR NOT NULL,
					config VARCHAR NOT NULL DEFAULT '{}',
					consent_category VARCHAR NOT NULL DEFAULT 'marketing',
					priority INTEGER DEFAULT 0,
					is_enabled INTEGER DEFAULT 1,
					version INTEGER DEFAULT 1,
					created_at BIGINT NOT NULL,
					updated_at BIGINT NOT NULL
				);
				CREATE INDEX IF NOT EXISTS idx_tm_tags_container ON tm_tags(container_id);

				CREATE TABLE IF NOT EXISTS tm_triggers (
					id VARCHAR PRIMARY KEY,
					container_id VARCHAR NOT NULL,
					name VARCHAR NOT NULL,
					trigger_type VARCHAR NOT NULL,
					config VARCHAR NOT NULL DEFAULT '{}',
					created_at BIGINT NOT NULL,
					updated_at BIGINT NOT NULL
				);
				CREATE INDEX IF NOT EXISTS idx_tm_triggers_container ON tm_triggers(container_id);

				CREATE TABLE IF NOT EXISTS tm_tag_triggers (
					tag_id VARCHAR NOT NULL,
					trigger_id VARCHAR NOT NULL,
					is_exception INTEGER DEFAULT 0,
					PRIMARY KEY (tag_id, trigger_id)
				);

				CREATE TABLE IF NOT EXISTS tm_variables (
					id VARCHAR PRIMARY KEY,
					container_id VARCHAR NOT NULL,
					name VARCHAR NOT NULL,
					variable_type VARCHAR NOT NULL,
					config VARCHAR NOT NULL DEFAULT '{}',
					created_at BIGINT NOT NULL,
					updated_at BIGINT NOT NULL
				);
				CREATE INDEX IF NOT EXISTS idx_tm_variables_container ON tm_variables(container_id);

				CREATE TABLE IF NOT EXISTS tm_snapshots (
					id VARCHAR PRIMARY KEY,
					container_id VARCHAR NOT NULL,
					version INTEGER NOT NULL,
					snapshot VARCHAR NOT NULL,
					published_by VARCHAR,
					published_at BIGINT NOT NULL,
					UNIQUE(container_id, version)
				);
			`,
		},
		{
			version: 14,
			sql: `
				-- Admin audit log for GDPR compliance
				CREATE TABLE IF NOT EXISTS audit_log (
					id VARCHAR PRIMARY KEY,
					timestamp BIGINT NOT NULL,
					user_id VARCHAR NOT NULL,
					user_email VARCHAR NOT NULL,
					action VARCHAR NOT NULL,
					resource_type VARCHAR NOT NULL,
					resource_id VARCHAR,
					detail VARCHAR,
					ip_address VARCHAR
				);

				CREATE INDEX IF NOT EXISTS idx_audit_log_timestamp ON audit_log(timestamp);
				CREATE INDEX IF NOT EXISTS idx_audit_log_user ON audit_log(user_id);
				CREATE INDEX IF NOT EXISTS idx_audit_log_action ON audit_log(action);
				CREATE INDEX IF NOT EXISTS idx_audit_log_resource ON audit_log(resource_type, resource_id);
			`,
		},
		{
			version: 15,
			sql: `
				-- Add bot detection columns to performance table
				ALTER TABLE performance ADD COLUMN bot_score INTEGER DEFAULT 0;
				ALTER TABLE performance ADD COLUMN bot_category VARCHAR DEFAULT 'human';

				CREATE INDEX IF NOT EXISTS idx_perf_bot_category ON performance(bot_category);
			`,
		},
		{
			version: 16,
			sql: `
				-- Seed default data retention setting
				INSERT INTO settings (key, value, updated_at)
				VALUES ('data_retention_days', '180', epoch_ms(now()))
				ON CONFLICT (key) DO NOTHING;
			`,
		},
		{
			version: 17,
			sql: `
				-- Ad platform connections (OAuth integrations)
				CREATE TABLE IF NOT EXISTS ad_connections (
					id VARCHAR PRIMARY KEY,
					provider VARCHAR NOT NULL,
					name VARCHAR NOT NULL,
					account_id VARCHAR,
					encrypted_tokens TEXT NOT NULL,
					status VARCHAR DEFAULT 'active',
					last_sync_at BIGINT,
					last_error TEXT,
					config TEXT DEFAULT '{}',
					created_by VARCHAR,
					created_at BIGINT NOT NULL,
					updated_at BIGINT NOT NULL
				);

				CREATE INDEX IF NOT EXISTS idx_ad_connections_provider ON ad_connections(provider);
				CREATE INDEX IF NOT EXISTS idx_ad_connections_status ON ad_connections(status);

				-- Daily campaign spend data from ad platforms
				CREATE TABLE IF NOT EXISTS ad_spend_daily (
					id VARCHAR PRIMARY KEY,
					connection_id VARCHAR NOT NULL,
					provider VARCHAR NOT NULL,
					date VARCHAR NOT NULL,
					campaign_id VARCHAR NOT NULL,
					campaign_name VARCHAR,
					cost_micros BIGINT NOT NULL DEFAULT 0,
					impressions INTEGER DEFAULT 0,
					clicks INTEGER DEFAULT 0,
					currency VARCHAR DEFAULT 'USD',
					created_at BIGINT NOT NULL
				);

				CREATE INDEX IF NOT EXISTS idx_ad_spend_date ON ad_spend_daily(date, provider);
				CREATE INDEX IF NOT EXISTS idx_ad_spend_campaign ON ad_spend_daily(campaign_id, date);
				CREATE INDEX IF NOT EXISTS idx_ad_spend_connection ON ad_spend_daily(connection_id);
			`,
		},
		{
			version: 18,
			sql: `
				-- Import jobs tracking
				CREATE TABLE IF NOT EXISTS import_jobs (
					id VARCHAR PRIMARY KEY,
					source VARCHAR NOT NULL,
					status VARCHAR NOT NULL DEFAULT 'pending',
					domain VARCHAR NOT NULL,
					file_name VARCHAR,
					file_size BIGINT DEFAULT 0,
					rows_total BIGINT DEFAULT 0,
					rows_imported BIGINT DEFAULT 0,
					rows_skipped BIGINT DEFAULT 0,
					error_message VARCHAR,
					column_mapping VARCHAR DEFAULT '{}',
					warnings VARCHAR DEFAULT '[]',
					started_at BIGINT,
					completed_at BIGINT,
					created_by VARCHAR NOT NULL,
					created_at BIGINT NOT NULL
				);

				-- Mark imported events for rollback
				ALTER TABLE events ADD COLUMN IF NOT EXISTS import_id VARCHAR;
				CREATE INDEX IF NOT EXISTS idx_events_import_id ON events(import_id);
			`,
		},
		{
			version: 19,
			sql: `
				-- Session recordings metadata
				CREATE TABLE IF NOT EXISTS session_recordings (
					session_id VARCHAR PRIMARY KEY,
					domain VARCHAR NOT NULL,
					visitor_hash VARCHAR NOT NULL,
					start_time BIGINT NOT NULL,
					duration BIGINT DEFAULT 0,
					pages INTEGER DEFAULT 1,
					first_url VARCHAR,
					device_type VARCHAR,
					browser_name VARCHAR,
					os_name VARCHAR,
					geo_country VARCHAR,
					screen_width INTEGER,
					screen_height INTEGER,
					size_bytes BIGINT DEFAULT 0,
					events_count INTEGER DEFAULT 0,
					status VARCHAR DEFAULT 'recording',
					created_at BIGINT NOT NULL,
					updated_at BIGINT NOT NULL
				);

				CREATE INDEX IF NOT EXISTS idx_recordings_domain_time ON session_recordings(domain, start_time);
				CREATE INDEX IF NOT EXISTS idx_recordings_visitor ON session_recordings(visitor_hash);
				CREATE INDEX IF NOT EXISTS idx_recordings_status ON session_recordings(status);

				-- Replay settings
				INSERT INTO settings (key, value, updated_at) VALUES
					('replay_enabled', 'false', epoch_ms(now())),
					('replay_sample_rate', '10', epoch_ms(now())),
					('replay_mask_text', 'true', epoch_ms(now())),
					('replay_mask_inputs', 'true', epoch_ms(now())),
					('replay_max_duration_sec', '1800', epoch_ms(now())),
					('replay_storage_quota_mb', '5120', epoch_ms(now()))
				ON CONFLICT (key) DO NOTHING;
			`,
		},
	}

	for _, m := range migrations {
		if m.version <= currentVersion {
			continue
		}

		tx, err := db.conn.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction for migration %d: %w", m.version, err)
		}

		_, err = tx.Exec(m.sql)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to run migration %d: %w", m.version, err)
		}

		_, err = tx.Exec("INSERT INTO migrations (version, applied_at) VALUES (?, epoch_ms(now()))", m.version)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record migration %d: %w", m.version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %d: %w", m.version, err)
		}
	}

	return nil
}
