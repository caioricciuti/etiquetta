package bot

import (
	"encoding/json"
	"strings"
)

// Bot categories
const (
	CategoryHuman      = "human"
	CategorySuspicious = "suspicious"
	CategoryBadBot     = "bad_bot"
	CategoryGoodBot    = "good_bot"
	CategoryAutomation = "automation"
	CategoryAICrawler  = "ai_crawler"
)

// Signal weights for bot scoring
const (
	WeightWebdriver       = 30 // navigator.webdriver detected
	WeightHeadlessBrowser = 25 // HeadlessChrome, Phantom
	WeightEmptyUA         = 20 // No User-Agent header
	WeightMissingHeaders  = 15 // No Accept-Language/Encoding
	WeightDatacenterIP    = 15 // AWS, GCP, Azure ranges
	WeightKnownBot        = 40 // Known bot UA (treated as good_bot)
	WeightAutomationUA    = 35 // puppeteer, selenium
	WeightShortUA         = 10 // <50 chars, no browser indicator
	WeightScreenAnomaly   = 15 // 0x0 or impossible values
	WeightNoPlugins       = 5  // No plugins detected
	WeightNoLanguages     = 5  // No languages array
	WeightSuspiciousPath   = 30 // Known attack/exploit path patterns
	WeightCDPDetected      = 25 // ChromeDriver Protocol properties on document
	WeightDocHiddenAtLoad  = 5  // Document hidden when page loaded (background tab)
)

// Signal represents a detected bot signal
type Signal struct {
	Name   string `json:"name"`
	Weight int    `json:"weight"`
	Value  string `json:"value,omitempty"`
}

// ScoringResult contains the bot score and signals
type ScoringResult struct {
	Score    int      `json:"score"`
	Category string   `json:"category"`
	Signals  []Signal `json:"signals"`
	IsBot    bool     `json:"is_bot"`
}

// ClientSignals contains bot detection signals from the client
type ClientSignals struct {
	Webdriver      bool `json:"webdriver"`
	Phantom        bool `json:"phantom"`
	Selenium       bool `json:"selenium"`
	Headless       bool `json:"headless"`
	ScreenValid    bool `json:"screen_valid"`
	Plugins        int  `json:"plugins"`
	Languages      int  `json:"languages"`
	ScreenWidth    int  `json:"screen_width"`
	ScreenHeight   int  `json:"screen_height"`
	CDPDetected    bool `json:"cdp_detected"`
	DocHiddenAtLoad bool `json:"doc_hidden_at_load"`
}

// CalculateScore computes the bot score based on various signals
func CalculateScore(userAgent string, clientSignals *ClientSignals, isDatacenterIP bool, isAICrawlerIP bool, aiCrawlerIPName string, headers map[string]string) *ScoringResult {
	result := &ScoringResult{
		Score:    0,
		Category: CategoryHuman,
		Signals:  make([]Signal, 0),
	}

	ua := strings.ToLower(userAgent)

	// Check for AI crawlers first (before good bots to prevent Applebot-Extended matching applebot)
	if IsAICrawler(userAgent) {
		result.Score = 0
		result.Category = CategoryAICrawler
		result.Signals = append(result.Signals, Signal{
			Name:   "known_ai_crawler",
			Weight: 0,
			Value:  GetAICrawlerName(userAgent),
		})
		result.IsBot = true
		return result
	}

	// Check AI crawler IP ranges (catches crawlers with spoofed/generic UAs)
	if isAICrawlerIP && aiCrawlerIPName != "" {
		result.Score = 0
		result.Category = CategoryAICrawler
		result.Signals = append(result.Signals, Signal{
			Name:   "ai_crawler_ip",
			Weight: 0,
			Value:  aiCrawlerIPName,
		})
		result.IsBot = true
		return result
	}

	// Check for known good bots
	if IsGoodBot(userAgent) {
		result.Score = 0
		result.Category = CategoryGoodBot
		result.Signals = append(result.Signals, Signal{
			Name:   "known_good_bot",
			Weight: 0,
			Value:  GetGoodBotName(userAgent),
		})
		result.IsBot = true
		return result
	}

	// Check User-Agent signals
	if userAgent == "" {
		result.Score += WeightEmptyUA
		result.Signals = append(result.Signals, Signal{Name: "empty_ua", Weight: WeightEmptyUA})
	} else {
		// Check for automation tools
		automationPatterns := []string{"puppeteer", "selenium", "webdriver", "playwright", "cypress"}
		for _, pattern := range automationPatterns {
			if strings.Contains(ua, pattern) {
				result.Score += WeightAutomationUA
				result.Signals = append(result.Signals, Signal{Name: "automation_ua", Weight: WeightAutomationUA, Value: pattern})
				break
			}
		}

		// Check for headless browsers
		if strings.Contains(ua, "headlesschrome") || strings.Contains(ua, "phantomjs") {
			result.Score += WeightHeadlessBrowser
			result.Signals = append(result.Signals, Signal{Name: "headless_browser", Weight: WeightHeadlessBrowser})
		}

		// Check for short/suspicious UA
		if len(userAgent) < 50 && !hasBrowserIndicator(ua) {
			result.Score += WeightShortUA
			result.Signals = append(result.Signals, Signal{Name: "short_ua", Weight: WeightShortUA})
		}
	}

	// Check client-side signals
	if clientSignals != nil {
		if clientSignals.Webdriver {
			result.Score += WeightWebdriver
			result.Signals = append(result.Signals, Signal{Name: "webdriver", Weight: WeightWebdriver})
		}

		if clientSignals.Phantom {
			result.Score += WeightHeadlessBrowser
			result.Signals = append(result.Signals, Signal{Name: "phantom", Weight: WeightHeadlessBrowser})
		}

		if clientSignals.Selenium {
			result.Score += WeightAutomationUA
			result.Signals = append(result.Signals, Signal{Name: "selenium", Weight: WeightAutomationUA})
		}

		if clientSignals.Headless {
			result.Score += WeightHeadlessBrowser
			result.Signals = append(result.Signals, Signal{Name: "headless", Weight: WeightHeadlessBrowser})
		}

		// Screen anomaly detection - only flag when both dimensions are zero (headless/bot)
		if clientSignals.ScreenWidth == 0 && clientSignals.ScreenHeight == 0 {
			result.Score += WeightScreenAnomaly
			result.Signals = append(result.Signals, Signal{Name: "screen_anomaly", Weight: WeightScreenAnomaly})
		}

		// No plugins (common in headless browsers)
		if clientSignals.Plugins == 0 {
			result.Score += WeightNoPlugins
			result.Signals = append(result.Signals, Signal{Name: "no_plugins", Weight: WeightNoPlugins})
		}

		// No languages
		if clientSignals.Languages == 0 {
			result.Score += WeightNoLanguages
			result.Signals = append(result.Signals, Signal{Name: "no_languages", Weight: WeightNoLanguages})
		}

		// CDP (ChromeDriver Protocol) detected
		if clientSignals.CDPDetected {
			result.Score += WeightCDPDetected
			result.Signals = append(result.Signals, Signal{Name: "cdp_detected", Weight: WeightCDPDetected})
		}

		// Document hidden at load (AI tools often open background tabs)
		if clientSignals.DocHiddenAtLoad {
			result.Score += WeightDocHiddenAtLoad
			result.Signals = append(result.Signals, Signal{Name: "doc_hidden_at_load", Weight: WeightDocHiddenAtLoad})
		}
	}

	// Check for datacenter IP
	if isDatacenterIP {
		result.Score += WeightDatacenterIP
		result.Signals = append(result.Signals, Signal{Name: "datacenter_ip", Weight: WeightDatacenterIP})
	}

	// Check for missing headers
	if headers != nil {
		if _, ok := headers["Accept-Language"]; !ok {
			result.Score += WeightMissingHeaders
			result.Signals = append(result.Signals, Signal{Name: "missing_accept_language", Weight: WeightMissingHeaders})
		}
	}

	// Cap score at 100
	if result.Score > 100 {
		result.Score = 100
	}

	// Determine category based on score
	result.Category = ScoreToCategory(result.Score)
	result.IsBot = result.Score > 50

	// Override category to "automation" when automation-specific signals are present
	if result.Score > 50 && clientSignals != nil {
		if clientSignals.CDPDetected || clientSignals.Webdriver || clientSignals.Selenium {
			result.Category = CategoryAutomation
		}
	}

	return result
}

// ScoreToCategory converts a score to a category
func ScoreToCategory(score int) string {
	switch {
	case score <= 20:
		return CategoryHuman
	case score <= 50:
		return CategorySuspicious
	default:
		return CategoryBadBot
	}
}

// SignalsToJSON converts signals to JSON string
func SignalsToJSON(signals []Signal) string {
	data, err := json.Marshal(signals)
	if err != nil {
		return "[]"
	}
	return string(data)
}

// Suspicious path prefixes commonly targeted by scanners and bots
var suspiciousPathPrefixes = []string{
	"/wp-admin", "/wp-login", "/wp-includes", "/wp-content/uploads",
	"/xmlrpc.php",
	"/.env", "/.git", "/.svn", "/.htaccess", "/.htpasswd",
	"/cgi-bin", "/shell", "/cmd", "/eval",
	"/phpmyadmin", "/pma", "/myadmin",
	"/admin/config", "/backup", "/dump",
}

// ScoreSuspiciousPath checks if a URL path matches known attack patterns
func ScoreSuspiciousPath(path string) *Signal {
	if path == "" {
		return nil
	}
	p := strings.ToLower(path)

	// Check path prefixes
	for _, prefix := range suspiciousPathPrefixes {
		if strings.HasPrefix(p, prefix) {
			return &Signal{Name: "suspicious_path", Weight: WeightSuspiciousPath, Value: prefix}
		}
	}

	// Check for .php extension (on non-PHP analytics sites, this is always a probe)
	if strings.HasSuffix(p, ".php") {
		return &Signal{Name: "suspicious_path", Weight: WeightSuspiciousPath, Value: ".php"}
	}

	return nil
}

// hasBrowserIndicator checks if UA contains browser indicators
func hasBrowserIndicator(ua string) bool {
	indicators := []string{"mozilla", "chrome", "safari", "firefox", "edge", "opera"}
	for _, indicator := range indicators {
		if strings.Contains(ua, indicator) {
			return true
		}
	}
	return false
}
