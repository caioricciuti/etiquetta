package bot

import (
	"net"
	"sync"
)

// AICrawlerIPRange maps CIDR ranges to AI provider names
type AICrawlerIPRange struct {
	CIDR     *net.IPNet
	Provider string
}

// AICrawlerIPDetector detects if an IP belongs to a known AI crawler provider
type AICrawlerIPDetector struct {
	mu     sync.RWMutex
	ranges []AICrawlerIPRange
}

// NewAICrawlerIPDetector creates a new AI crawler IP detector
func NewAICrawlerIPDetector() *AICrawlerIPDetector {
	d := &AICrawlerIPDetector{}
	d.loadRanges()
	return d
}

// loadRanges loads known AI crawler IP ranges from published sources.
// These should be periodically verified against official documentation:
//   - OpenAI: https://openai.com/gptbot-ranges.txt
//   - Google: https://developers.google.com/search/docs/crawling-indexing/verifying-googlebot
//   - Apple: https://support.apple.com/en-us/111325
func (d *AICrawlerIPDetector) loadRanges() {
	d.mu.Lock()
	defer d.mu.Unlock()

	type entry struct {
		cidr     string
		provider string
	}

	entries := []entry{
		// OpenAI (GPTBot, ChatGPT-User, OAI-SearchBot)
		// Source: openai.com/gptbot-ranges.txt (Azure-hosted)
		{"20.15.240.64/28", "OpenAI"},
		{"20.15.240.80/28", "OpenAI"},
		{"20.15.240.96/28", "OpenAI"},
		{"20.15.240.176/28", "OpenAI"},
		{"20.171.206.0/24", "OpenAI"},
		{"52.230.152.0/24", "OpenAI"},
		{"52.233.106.0/24", "OpenAI"},

		// Google (Google-Extended, Googlebot-shared ranges)
		// Source: developers.google.com/search/docs/crawling-indexing/verifying-googlebot
		{"66.249.64.0/19", "Google"},
		{"64.233.160.0/19", "Google"},
		{"72.14.192.0/18", "Google"},

		// Apple (Applebot, Applebot-Extended)
		// Source: Apple's AS714 allocation
		{"17.0.0.0/8", "Apple"},

		// Common Crawl (CCBot) — partial, AWS-hosted
		{"44.201.72.0/22", "CommonCrawl"},

		// Perplexity — known ranges
		{"52.152.0.0/14", "Perplexity"},
	}

	d.ranges = make([]AICrawlerIPRange, 0, len(entries))
	for _, e := range entries {
		_, ipNet, err := net.ParseCIDR(e.cidr)
		if err == nil {
			d.ranges = append(d.ranges, AICrawlerIPRange{CIDR: ipNet, Provider: e.provider})
		}
	}
}

// Check returns whether the IP belongs to a known AI crawler range and which provider
func (d *AICrawlerIPDetector) Check(ipStr string) (bool, string) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false, ""
	}

	for _, r := range d.ranges {
		if r.CIDR.Contains(ip) {
			return true, r.Provider
		}
	}

	return false, ""
}

// Global instance
var defaultAICrawlerDetector *AICrawlerIPDetector
var aiCrawlerDetectorOnce sync.Once

// IsAICrawlerIP checks if IP belongs to a known AI crawler provider (uses global instance)
func IsAICrawlerIP(ip string) (bool, string) {
	aiCrawlerDetectorOnce.Do(func() {
		defaultAICrawlerDetector = NewAICrawlerIPDetector()
	})
	return defaultAICrawlerDetector.Check(ip)
}
