package enrichment

import (
	"crypto/md5"
	"encoding/hex"
	"net"
	"net/url"
	"strings"

	"github.com/caioricciuti/etiquetta/internal/bot"
)

// Enricher provides event enrichment
type Enricher struct {
	geoIP *GeoIP
}

// New creates a new Enricher
func New(geoipPath string) *Enricher {
	geoIP, _ := NewGeoIP(geoipPath)
	return &Enricher{geoIP: geoIP}
}

// ReloadGeoIP reloads the GeoIP database from a new path
func (e *Enricher) ReloadGeoIP(path string) error {
	if e.geoIP != nil {
		e.geoIP.Close()
	}
	geoIP, err := NewGeoIP(path)
	if err != nil {
		return err
	}
	e.geoIP = geoIP
	return nil
}

// EnrichmentResult contains enriched data
type EnrichmentResult struct {
	// Geo
	GeoCountry   string
	GeoCity      string
	GeoRegion    string
	GeoLatitude  float64
	GeoLongitude float64

	// Device
	BrowserName string
	OSName      string
	DeviceType  string
	IsBot       bool

	// Bot scoring
	BotScore        int
	BotCategory     string
	BotSignals      string
	DatacenterIP    bool
	AICrawlerIP     bool
	AICrawlerIPName string

	// Referrer
	ReferrerDomain string
	ReferrerType   string
}

// Enrich processes an event with additional data
func (e *Enricher) Enrich(ip, userAgent, referrerURL string) *EnrichmentResult {
	return e.EnrichWithHeaders(ip, userAgent, referrerURL, nil)
}

// EnrichWithHeaders processes an event with additional data including headers
func (e *Enricher) EnrichWithHeaders(ip, userAgent, referrerURL string, headers map[string]string) *EnrichmentResult {
	result := &EnrichmentResult{}

	// GeoIP lookup
	if e.geoIP != nil {
		if geo := e.geoIP.Lookup(ip); geo != nil {
			result.GeoCountry = geo.Country
			result.GeoCity = geo.City
			result.GeoRegion = geo.Region
			result.GeoLatitude = geo.Latitude
			result.GeoLongitude = geo.Longitude
		}
	}

	// User-Agent parsing
	ua := ParseUserAgent(userAgent)
	result.BrowserName = ua.BrowserName
	result.OSName = ua.OSName
	result.DeviceType = ua.DeviceType

	// Check datacenter IP
	result.DatacenterIP = bot.IsDatacenterIP(ip)

	// Check AI crawler IP ranges
	result.AICrawlerIP, result.AICrawlerIPName = bot.IsAICrawlerIP(ip)

	// Bot scoring (server-side, without client signals)
	// Client signals will be added in handlers.go
	botResult := bot.CalculateScore(userAgent, nil, result.DatacenterIP, result.AICrawlerIP, result.AICrawlerIPName, headers)
	result.BotScore = botResult.Score
	result.BotCategory = botResult.Category
	result.BotSignals = bot.SignalsToJSON(botResult.Signals)
	result.IsBot = botResult.IsBot

	// Referrer classification
	if referrerURL != "" {
		result.ReferrerDomain = extractDomain(referrerURL)
		result.ReferrerType = classifyReferrerInternal(referrerURL, result.ReferrerDomain)
	}

	return result
}

// ExtractClientIP gets the real client IP from request headers
func ExtractClientIP(remoteAddr string, headers map[string]string) string {
	// Check X-Forwarded-For first
	if xff, ok := headers["X-Forwarded-For"]; ok && xff != "" {
		// First IP in the list is the original client
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}

	// Check X-Real-IP
	if xri, ok := headers["X-Real-IP"]; ok && xri != "" {
		return xri
	}

	// Fall back to remote address
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

// HashError creates a hash for error deduplication
func HashError(errorType, errorMessage, scriptURL string, lineNumber int) string {
	data := errorType + "|" + errorMessage + "|" + scriptURL + "|" + string(rune(lineNumber))
	hash := md5.Sum([]byte(data))
	return hex.EncodeToString(hash[:8])
}

func extractDomain(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Host
}

// ClassifyReferrer classifies a referrer URL into categories (exported for use in handlers)
func ClassifyReferrer(referrerURL string) string {
	if referrerURL == "" {
		return "direct"
	}
	domain := extractDomain(referrerURL)
	return classifyReferrerInternal(referrerURL, domain)
}

func classifyReferrerInternal(referrerURL, referrerDomain string) string {
	if referrerURL == "" {
		return "direct"
	}

	domain := strings.ToLower(referrerDomain)

	// Search engines
	searchEngines := []string{
		"google.", "bing.", "yahoo.", "duckduckgo.", "baidu.",
		"yandex.", "ask.", "ecosia.", "aol.",
	}
	for _, se := range searchEngines {
		if strings.Contains(domain, se) {
			return "search"
		}
	}

	// Social networks
	socialNetworks := []string{
		"facebook.", "twitter.", "t.co", "linkedin.", "instagram.",
		"pinterest.", "reddit.", "youtube.", "tiktok.", "snapchat.",
		"whatsapp.", "telegram.", "discord.",
	}
	for _, sn := range socialNetworks {
		if strings.Contains(domain, sn) {
			return "social"
		}
	}

	// Check for UTM campaign
	parsed, err := url.Parse(referrerURL)
	if err == nil && parsed.Query().Get("utm_source") != "" {
		return "campaign"
	}

	return "external"
}
