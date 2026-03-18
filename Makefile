.PHONY: build run dev clean test ui all install release

# Version info (reads from VERSION file)
VERSION = $(shell cat VERSION 2>/dev/null || echo "dev")
COMMIT = $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE = $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS = -ldflags "-s -w -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildDate=$(BUILD_DATE)"

# Build everything (UI + binary)
all: ui build

# Build the Go binary (CGO required for DuckDB)
build:
	CGO_ENABLED=1 go build $(LDFLAGS) -o bin/etiquetta ./cmd/etiquetta

# Run the server
run: build
	./bin/etiquetta serve

# Initialize the database and create admin user
init: build
	./bin/etiquetta init

# Development mode with hot reload (requires air)
dev:
	air

# Build UI with bun
ui:
	cd ui && bun install && bun run build
	cp ui/node_modules/rrweb/dist/rrweb.umd.min.cjs internal/api/rrweb.min.js

# Clean build artifacts
clean:
	rm -rf bin/
	rm -rf ui/dist/
	rm -f internal/api/rrweb.min.js

# Run tests
test:
	CGO_ENABLED=1 go test -v ./...

# Install to /usr/local/bin
install: all
	cp bin/etiquetta /usr/local/bin/etiquetta

# Generate license keypair
keygen:
	go run ./cmd/licensegen -keygen

# Generate a test pro license (requires keypair)
license-pro:
	go run ./cmd/licensegen -licensee "Development" -tier pro -days 365

# Generate a test enterprise license (requires keypair)
license-enterprise:
	go run ./cmd/licensegen -licensee "Development" -tier enterprise -days 365

# Download GeoIP database (requires credentials in settings)
geoip:
	./bin/etiquetta geoip download

# Build release binaries for all platforms
release: ui
	@mkdir -p dist
	@echo "Building release binaries..."
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/etiquetta-linux-amd64 ./cmd/etiquetta
	CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o dist/etiquetta-linux-arm64 ./cmd/etiquetta
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o dist/etiquetta-darwin-amd64 ./cmd/etiquetta
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o dist/etiquetta-darwin-arm64 ./cmd/etiquetta
	@echo "Release binaries created in dist/"

# Show help
help:
	@echo "Etiquetta Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make all       - Build UI and binary"
	@echo "  make build     - Build Go binary only"
	@echo "  make ui        - Build React UI only"
	@echo "  make run       - Build and run server"
	@echo "  make init      - Initialize database and create admin"
	@echo "  make dev       - Development mode with hot reload"
	@echo "  make test      - Run tests"
	@echo "  make clean     - Remove build artifacts"
	@echo "  make install   - Install to /usr/local/bin"
	@echo "  make release   - Build binaries for all platforms"
	@echo "  make geoip     - Download GeoIP database"
	@echo ""
	@echo "CLI Commands:"
	@echo "  etiquetta serve     - Start the server"
	@echo "  etiquetta init      - Interactive setup wizard"
	@echo "  etiquetta version   - Show version info"
	@echo "  etiquetta user list - List users"
	@echo "  etiquetta user create - Create a user"
	@echo "  etiquetta geoip download - Download GeoIP database"
