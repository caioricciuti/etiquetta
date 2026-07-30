<p align="center">
  <img src="ui/public/logo-complete.png" alt="Etiquetta" width="120" >
</p>

<p align="center">
  <strong>Self-hosted web analytics. Privacy-focused. Single binary.</strong>
</p>

<p align="center">
  <a href="https://github.com/caioricciuti/etiquetta/actions/workflows/ci.yml">
    <img src="https://github.com/caioricciuti/etiquetta/actions/workflows/ci.yml/badge.svg" alt="CI">
  </a>
  <a href="https://github.com/caioricciuti/etiquetta/releases/latest">
    <img src="https://img.shields.io/github/v/release/caioricciuti/etiquetta" alt="Release">
  </a>
  <a href="LICENSE">
    <img src="https://img.shields.io/badge/license-GPL--3.0-blue.svg" alt="License">
  </a>
</p>

---

## Quick Install

```bash
curl -sSL https://raw.githubusercontent.com/caioricciuti/etiquetta/main/install.sh | bash
```

Or with systemd service (Linux):

```bash
curl -sSL https://raw.githubusercontent.com/caioricciuti/etiquetta/main/install.sh | bash -s -- --with-systemd
```

## Features

- **Single Binary** - Go backend with embedded UI, no external dependencies
- **DuckDB Storage** - Embedded analytical storage in the same self-hosted data directory
- **Privacy-First** - Server-side session computation, no third-party cookies
- **Multi-Domain** - Track multiple websites from one installation
- **Real-Time Analytics** - Pageviews, visitors, referrers, and more
- **Bot Detection** - Identify and filter automated traffic
- **Core Web Vitals** - LCP, FCP, CLS, INP, TTFB metrics (Pro)
- **Error Tracking** - Capture JavaScript errors with deduplication (Pro)
- **Ad Fraud Detection** - Detect suspicious traffic patterns (Pro)
- **Dark Mode** - Beautiful UI that respects your system preference

## Quick Start

```bash
# Download the binary (or use the install script above)
# Then run:
./etiquetta serve

# Open http://localhost:3456
# Complete the setup wizard to create your admin account
```

## Uninstall

### Complete removal (systemd install)

```bash
# Stop and disable the service
sudo systemctl stop etiquetta
sudo systemctl disable etiquetta

# Remove service file
sudo rm /etc/systemd/system/etiquetta.service
sudo systemctl daemon-reload

# Remove binary
sudo rm /usr/local/bin/etiquetta

# Remove data (WARNING: deletes all analytics data!)
sudo rm -rf /var/lib/etiquetta

# Remove etiquetta user (optional)
sudo userdel etiquetta
```

### Manual install removal

```bash
# Just remove the binary and data
rm ./bin/etiquetta
rm -rf ./data
```

## Build from Source

Requires Go 1.24+ and Bun.

```bash
git clone https://github.com/caioricciuti/etiquetta.git
cd etiquetta
make all
./bin/etiquetta serve
```

## Nginx Setup (Reverse Proxy)

Copy the example config:

```bash
sudo cp nginx.conf.example /etc/nginx/sites-available/etiquetta
sudo ln -s /etc/nginx/sites-available/etiquetta /etc/nginx/sites-enabled/
# Edit the file and replace 'your-domain.com' with your domain
sudo nano /etc/nginx/sites-available/etiquetta
sudo nginx -t && sudo systemctl reload nginx

# Add HTTPS with Let's Encrypt
sudo certbot --nginx -d your-domain.com
```

## Configuration

Environment variables (or `.env` file):

| Variable                  | Default  | Description                                                     |
| ------------------------- | -------- | --------------------------------------------------------------- |
| `ETIQUETTA_PORT`          | `3456`   | HTTP server port                                                |
| `ETIQUETTA_DATA_DIR`      | `./data` | Database storage directory                                      |
| `ETIQUETTA_SECURE_COOKIES`| `false`  | Set to `true` whenever users access Etiquetta over HTTPS, including behind a reverse proxy |
| `ETIQUETTA_STORAGE`       | `duckdb` | Set to `ducklake` to store event data in a DuckLake catalog (see below) |

## DuckLake Storage (experimental, opt-in)

By default Etiquetta stores everything in a single DuckDB file. Setting
`ETIQUETTA_STORAGE=ducklake` moves the append-only event tables (`events`,
`performance`, `errors`) into a [DuckLake](https://ducklake.select) catalog under
`{data-dir}/lake` — Parquet data files plus a SQL catalog with snapshots and
time-travel. Metadata (domains, users, settings) stays in the DuckDB file.

Why: far more compact storage, native snapshot/time-travel history, and it
removes the buffer→parquet→load ingestion path that is the historical source of
storage bugs. All dashboard queries work unchanged (a main-first `search_path`
routes event tables to the lake).

**Enabling it is a one-way migration** — the event tables are copied into the
lake and dropped from the DuckDB file. Before the first transition Etiquetta
automatically writes a rollback snapshot to `{data-dir}/etiquetta.duckdb.pre-ducklake`.

```bash
# 1. (recommended) take a backup first
etiquetta backup --output ./etiquetta-backup.tar.zst
# 2. stop, enable, start
etiquetta stop
ETIQUETTA_STORAGE=ducklake etiquetta serve   # or add it to your .env / service
```

To roll back: stop the server, restore `etiquetta.duckdb.pre-ducklake` over
`etiquetta.duckdb`, remove `{data-dir}/lake`, and unset `ETIQUETTA_STORAGE`.

The extensions (`ducklake`, `httpfs`, `icu`) are installed on first run and
require network access at that point.

## Production Backups

Backups are offline and non-destructive. Stop Etiquetta, keep it stopped for the
whole command, and write the archive outside the data directory:

```bash
sudo systemctl stop etiquetta
sudo install -d -o etiquetta -g etiquetta -m 0700 /var/backups/etiquetta
sudo -u etiquetta etiquetta --data /var/lib/etiquetta backup \
  --output /var/backups/etiquetta/etiquetta-$(date -u +%Y%m%dT%H%M%SZ).tar.gz
```

The command refuses to run while DuckDB is in use, checkpoints the database,
archives the complete data directory, and writes both an embedded manifest and a
`.sha256` sidecar. Store both files in a separate, encrypted location. Test a
restore on a staging instance before relying on the backup for an upgrade.
See the [production upgrade and rollback runbook](docs/production-upgrade.md)
before changing a live installation.

## Tracking Setup

### 1. Add Your Domain

1. Log in to Etiquetta
2. Go to **Settings > Domains**
3. Click **Add Domain** and enter your site name and domain

### 2. Install the Tracking Script

Add this snippet to your website's `<head>`:

```html
<script defer data-site="YOUR_SITE_ID" src="https://your-etiquetta-instance.com/s.js?id=YOUR_SITE_ID"></script>
```

The tracker automatically collects:

- Pageviews with SPA navigation support
- Unique visitors (fingerprint-based in cookieless mode, or persistent identifiers when configured)
- Referrer information
- Core Web Vitals (LCP, FCP, CLS, INP, TTFB)
- JavaScript errors
- Scroll depth (25%, 50%, 75%, 100%)
- Outbound link clicks
- Engagement time
- Bot detection signals

### Privacy configuration

- **DNT/GPC**: The analytics tracker can honor browser privacy signals.
- **Visitor identification**: Cookie, local-storage, and cookieless modes have different privacy and continuity trade-offs.
- **Data ownership**: Analytics data is stored on your Etiquetta server, while configured tags and map/editor integrations may contact third parties.
- **Consent and legal basis**: Cookieless tracking is not automatically exempt from consent or data-protection requirements. Configure each property for its jurisdiction and obtain legal advice for your use case.

## Pricing

| Feature            | Community |     Pro      |  Enterprise   |
| ------------------ | :-------: | :----------: | :-----------: |
| Core Analytics     |     ✓     |      ✓       |       ✓       |
| Unlimited Domains  |     ✓     |      ✓       |       ✓       |
| Bot Analysis       |     ✓     |      ✓       |       ✓       |
| Core Web Vitals    |     -     |      ✓       |       ✓       |
| Error Tracking     |     -     |      ✓       |       ✓       |
| Data Export        |     -     |      ✓       |       ✓       |
| Ad Fraud Detection |     -     |      -       |       ✓       |
| Multi-User         |     -     |      -       |       ✓       |
| Priority Support   |     -     |      -       |       ✓       |
| **Price**          | **Free**  | **€99/year** | **€299/year** |

[Get a License](https://etiquetta.com/pricing)

## API Reference

### Authentication

All API endpoints (except `/api/auth/setup` and `/api/auth/login`) require authentication via HTTP-only cookie.

```
POST /api/auth/setup    - Initial admin account creation (first run only)
POST /api/auth/login    - Login with email/password
POST /api/auth/logout   - Clear session
GET  /api/auth/me       - Get current user info
POST /api/auth/password - Change password
```

### Domains

```
GET    /api/domains              - List all registered domains
POST   /api/domains              - Add a new domain
DELETE /api/domains/{id}         - Remove a domain
GET    /api/domains/{id}/snippet - Get tracking snippet for a domain
```

### Analytics

```
GET /api/stats/overview     - Summary stats
GET /api/stats/timeseries   - Pageviews over time
GET /api/stats/pages        - Top pages
GET /api/stats/referrers    - Top referrers
GET /api/stats/devices      - Device breakdown
GET /api/stats/geo          - Geographic breakdown
GET /api/stats/vitals       - Core Web Vitals (Pro)
GET /api/stats/errors       - JavaScript errors (Pro)
GET /api/stats/bots         - Bot traffic breakdown
GET /api/stats/fraud        - Fraud analysis (Enterprise)
```

Query parameters: `?from=2024-01-01&to=2024-01-31&domain=example.com`

### Event Ingestion

```
POST /i     - Receive tracking events (NDJSON format)
GET  /s.js  - Serve tracker script
```

## Development

```bash
# Start development server with hot reload
make dev

# Run tests
make test

# Build for all platforms
make release

# Clean build artifacts
make clean
```

## Architecture

```
etiquetta/
├── cmd/etiquetta/    # CLI entry point
├── internal/
│   ├── api/          # HTTP handlers and router
│   ├── auth/         # JWT authentication
│   ├── database/     # DuckDB with transactional migrations
│   ├── enrichment/   # GeoIP, bot detection
│   └── licensing/    # License verification
├── ui/               # React frontend (Vite + shadcn)
│   ├── src/
│   │   ├── components/
│   │   ├── hooks/
│   │   └── pages/
│   └── dist/         # Built UI (embedded in binary)
└── data/             # DuckDB database and local runtime state
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run `make test`
5. Submit a pull request

## License

Etiquetta is licensed under the [GNU General Public License v3.0](LICENSE).

You are free to use, modify, and distribute this software under the terms of the GPL-3.0 license. If you distribute modified versions, you must also make the source code available under the same license.

## Support

- Issues: [github.com/caioricciuti/etiquetta/issues](https://github.com/caioricciuti/etiquetta/issues)
- Website: [etiquetta.com](https://etiquetta.com)

---

<p align="center">
  Built with Go and React. Privacy-focused analytics for everyone.
</p>
