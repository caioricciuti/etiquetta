package buffer

import (
	"time"

	"github.com/caioricciuti/etiquetta/internal/database"
)

// Event is the parquet-friendly representation of a tracking event.
// All fields are non-pointer types for parquet serialization.
type Event struct {
	ID           string  `parquet:"id"`
	Timestamp    int64   `parquet:"timestamp"`
	EventType    string  `parquet:"event_type"`
	EventName    string  `parquet:"event_name"`
	SessionID    string  `parquet:"session_id"`
	VisitorHash  string  `parquet:"visitor_hash"`
	Domain       string  `parquet:"domain"`
	URL          string  `parquet:"url"`
	Path         string  `parquet:"path"`
	PageTitle    string  `parquet:"page_title"`
	ReferrerURL  string  `parquet:"referrer_url"`
	ReferrerType string  `parquet:"referrer_type"`
	UTMSource    string  `parquet:"utm_source"`
	UTMMedium    string  `parquet:"utm_medium"`
	UTMCampaign  string  `parquet:"utm_campaign"`
	GeoCountry   string  `parquet:"geo_country"`
	GeoCity      string  `parquet:"geo_city"`
	GeoRegion    string  `parquet:"geo_region"`
	GeoLatitude  float64 `parquet:"geo_latitude"`
	GeoLongitude float64 `parquet:"geo_longitude"`
	BrowserName  string  `parquet:"browser_name"`
	OSName       string  `parquet:"os_name"`
	DeviceType   string  `parquet:"device_type"`
	IsBot        int64   `parquet:"is_bot"`
	Props        string  `parquet:"props"`
	BotScore     int64   `parquet:"bot_score"`
	BotSignals   string  `parquet:"bot_signals"`
	BotCategory  string  `parquet:"bot_category"`
	HasScroll    int64   `parquet:"has_scroll"`
	HasMouseMove int64   `parquet:"has_mouse_move"`
	HasClick     int64   `parquet:"has_click"`
	HasTouch     int64   `parquet:"has_touch"`
	ClickX       int64   `parquet:"click_x"`
	ClickY       int64   `parquet:"click_y"`
	PageDuration int64   `parquet:"page_duration"`
	DatacenterIP int64   `parquet:"datacenter_ip"`
	IPHash       string  `parquet:"ip_hash"`
}

// Performance is the parquet-friendly representation of web vitals.
type Performance struct {
	ID             string  `parquet:"id"`
	Timestamp      int64   `parquet:"timestamp"`
	SessionID      string  `parquet:"session_id"`
	VisitorHash    string  `parquet:"visitor_hash"`
	Domain         string  `parquet:"domain"`
	URL            string  `parquet:"url"`
	Path           string  `parquet:"path"`
	LCP            float64 `parquet:"lcp"`
	CLS            float64 `parquet:"cls"`
	FCP            float64 `parquet:"fcp"`
	TTFB           float64 `parquet:"ttfb"`
	INP            float64 `parquet:"inp"`
	PageLoadTime   float64 `parquet:"page_load_time"`
	DeviceType     string  `parquet:"device_type"`
	ConnectionType string  `parquet:"connection_type"`
	GeoCountry     string  `parquet:"geo_country"`
}

// ErrorEvent is the parquet-friendly representation of a JS error.
type ErrorEvent struct {
	ID           string `parquet:"id"`
	Timestamp    int64  `parquet:"timestamp"`
	SessionID    string `parquet:"session_id"`
	VisitorHash  string `parquet:"visitor_hash"`
	Domain       string `parquet:"domain"`
	URL          string `parquet:"url"`
	Path         string `parquet:"path"`
	ErrorType    string `parquet:"error_type"`
	ErrorMessage string `parquet:"error_message"`
	ErrorStack   string `parquet:"error_stack"`
	ErrorHash    string `parquet:"error_hash"`
	ScriptURL    string `parquet:"script_url"`
	LineNumber   int64  `parquet:"line_number"`
	ColumnNumber int64  `parquet:"column_number"`
	BrowserName  string `parquet:"browser_name"`
	GeoCountry   string `parquet:"geo_country"`
}

// VisitorSession is the parquet-friendly representation of a materialized session.
type VisitorSession struct {
	ID          string `parquet:"id"`
	SessionID   string `parquet:"session_id"`
	VisitorHash string `parquet:"visitor_hash"`
	Domain      string `parquet:"domain"`
	StartTime   int64  `parquet:"start_time"`
	EndTime     int64  `parquet:"end_time"`
	Duration    int64  `parquet:"duration"`
	Pageviews   int64  `parquet:"pageviews"`
	EntryURL    string `parquet:"entry_url"`
	ExitURL     string `parquet:"exit_url"`
	IsBounce    int64  `parquet:"is_bounce"`
	BotScore    int64  `parquet:"bot_score"`
	BotCategory string `parquet:"bot_category"`
}

// --- Conversion functions ---

func strFromPtr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func float64FromPtr(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func intFromPtr(p *int) int64 {
	if p == nil {
		return 0
	}
	return int64(*p)
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func timeToMs(t time.Time) int64 {
	return t.UnixMilli()
}

// ConvertEvent converts a database.Event to a buffer.Event.
func ConvertEvent(e *database.Event) Event {
	props := "{}"
	if e.Props != nil {
		props = string(e.Props)
	}
	botSignals := "[]"
	if e.BotSignals != "" {
		botSignals = e.BotSignals
	}
	botCategory := "human"
	if e.BotCategory != "" {
		botCategory = e.BotCategory
	}

	return Event{
		ID:           e.ID,
		Timestamp:    timeToMs(e.Timestamp),
		EventType:    e.EventType,
		EventName:    strFromPtr(e.EventName),
		SessionID:    e.SessionID,
		VisitorHash:  e.VisitorHash,
		Domain:       e.Domain,
		URL:          e.URL,
		Path:         e.Path,
		PageTitle:    strFromPtr(e.PageTitle),
		ReferrerURL:  strFromPtr(e.ReferrerURL),
		ReferrerType: strFromPtr(e.ReferrerType),
		UTMSource:    strFromPtr(e.UTMSource),
		UTMMedium:    strFromPtr(e.UTMMedium),
		UTMCampaign:  strFromPtr(e.UTMCampaign),
		GeoCountry:   strFromPtr(e.GeoCountry),
		GeoCity:      strFromPtr(e.GeoCity),
		GeoRegion:    strFromPtr(e.GeoRegion),
		GeoLatitude:  float64FromPtr(e.GeoLatitude),
		GeoLongitude: float64FromPtr(e.GeoLongitude),
		BrowserName:  strFromPtr(e.BrowserName),
		OSName:       strFromPtr(e.OSName),
		DeviceType:   strFromPtr(e.DeviceType),
		IsBot:        boolToInt(e.IsBot),
		Props:        props,
		BotScore:     int64(e.BotScore),
		BotSignals:   botSignals,
		BotCategory:  botCategory,
		HasScroll:    boolToInt(e.HasScroll),
		HasMouseMove: boolToInt(e.HasMouseMove),
		HasClick:     boolToInt(e.HasClick),
		HasTouch:     boolToInt(e.HasTouch),
		ClickX:       intFromPtr(e.ClickX),
		ClickY:       intFromPtr(e.ClickY),
		PageDuration: intFromPtr(e.PageDuration),
		DatacenterIP: boolToInt(e.DatacenterIP),
		IPHash:       strFromPtr(e.IPHash),
	}
}

// ConvertPerformance converts a database.Performance to a buffer.Performance.
func ConvertPerformance(p *database.Performance) Performance {
	return Performance{
		ID:             p.ID,
		Timestamp:      timeToMs(p.Timestamp),
		SessionID:      p.SessionID,
		VisitorHash:    p.VisitorHash,
		Domain:         p.Domain,
		URL:            p.URL,
		Path:           p.Path,
		LCP:            float64FromPtr(p.LCP),
		CLS:            float64FromPtr(p.CLS),
		FCP:            float64FromPtr(p.FCP),
		TTFB:           float64FromPtr(p.TTFB),
		INP:            float64FromPtr(p.INP),
		PageLoadTime:   float64FromPtr(p.PageLoadTime),
		DeviceType:     strFromPtr(p.DeviceType),
		ConnectionType: strFromPtr(p.ConnectionType),
		GeoCountry:     strFromPtr(p.GeoCountry),
	}
}

// ConvertError converts a database.Error to a buffer.ErrorEvent.
func ConvertError(e *database.Error) ErrorEvent {
	return ErrorEvent{
		ID:           e.ID,
		Timestamp:    timeToMs(e.Timestamp),
		SessionID:    e.SessionID,
		VisitorHash:  e.VisitorHash,
		Domain:       e.Domain,
		URL:          e.URL,
		Path:         e.Path,
		ErrorType:    e.ErrorType,
		ErrorMessage: e.ErrorMessage,
		ErrorStack:   strFromPtr(e.ErrorStack),
		ErrorHash:    e.ErrorHash,
		ScriptURL:    strFromPtr(e.ScriptURL),
		LineNumber:   intFromPtr(e.LineNumber),
		ColumnNumber: intFromPtr(e.ColumnNumber),
		BrowserName:  strFromPtr(e.BrowserName),
		GeoCountry:   strFromPtr(e.GeoCountry),
	}
}
