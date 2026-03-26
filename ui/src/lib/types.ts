export interface ApiKey {
  id: string
  name: string
  key_prefix: string
  created_at: number
  last_used_at: number | null
  revoked_at: number | null
}

export interface OverviewStats {
  total_events: number
  unique_visitors: number
  sessions: number
  pageviews: number
  live_visitors: number
  bounce_rate: number
  avg_session_seconds: number
  prev_total_events?: number
  prev_unique_visitors?: number
  prev_sessions?: number
  prev_pageviews?: number
  prev_bounce_rate?: number
  prev_avg_session_seconds?: number
}

export interface TimeseriesPoint {
  period: string
  pageviews: number
  visitors: number
}

export interface TopPage {
  path: string
  views: number
  visitors: number
}

export interface Referrer {
  source: string
  referrer_type?: string
  visits: number
  visitors: number
}

export interface GeoData {
  country: string
  visitors: number
}

export interface DeviceData {
  device: string
  visitors: number
}

export interface BrowserData {
  browser: string
  visitors: number
}

export interface WebVitals {
  lcp: number
  cls: number
  fcp: number
  ttfb: number
  inp: number
  samples: number
}

export interface ErrorSummary {
  error_hash: string
  error_type: string
  error_message: string
  occurrences: number
  affected_sessions: number
}

export interface ErrorDetail {
  id: string
  timestamp: number
  session_id: string
  domain: string
  url: string
  path: string
  error_type: string
  error_message: string
  error_stack?: string
  error_hash: string
  script_url?: string
  line_number?: number
  column_number?: number
  browser_name?: string
  geo_country?: string
}

export interface ErrorTimeseries {
  period: string
  count: number
}

export interface Campaign {
  utm_source: string
  utm_medium: string
  utm_campaign: string
  visitors: number
  sessions: number
}

export interface CustomEvent {
  event_name: string
  count: number
  unique_visitors: number
}

export interface OutboundLink {
  url: string
  clicks: number
  unique_visitors: number
}

// Events page types
export interface EventSummaryItem {
  event_type: string
  event_name: string
  count: number
  unique_visitors: number
  sessions: number
}

export interface EventTypeCount {
  event_type: string
  count: number
}

export interface EventsSummaryResponse {
  events: EventSummaryItem[]
  total: number
  prev_total: number
  type_counts: EventTypeCount[]
}

export interface EventTimeseriesPoint {
  period: string
  count: number
  visitors: number
}

export interface EventPropValue {
  value: string
  count: number
}

export interface EventProperty {
  key: string
  count: number
  values: EventPropValue[]
}

export interface EventPropsResponse {
  properties: EventProperty[]
}

export interface MapPoint {
  city: string
  country: string
  lat: number
  lng: number
  visitors: number
  pageviews: number
}

// Bot Analysis types
export interface BotCategory {
  category: string
  events: number
  visitors: number
}

export interface ScoreDistribution {
  range: string
  count: number
}

export interface BotTimeseries {
  period: string
  humans: number
  suspicious: number
  bad_bots: number
  good_bots: number
  ai_crawlers: number
}

export interface AICrawlerSettings {
  known_crawlers: string[]
  rules: Record<string, 'allow' | 'block'>
}

export interface BotDetail {
  browser_name: string
  category: string
  score: number
  signals: string[]
  hits: number
  visitors: number
  sessions: number
  last_seen: number
}

export interface BotData {
  categories: BotCategory[]
  score_distribution: ScoreDistribution[]
  timeseries: BotTimeseries[]
  top_bots: BotDetail[]
}

// Calendar heatmap
export interface CalendarHeatmapPoint {
  date: string
  sessions: number
}

// Ad Fraud types
export interface FraudSignal {
  type: string
  description: string
  count: number
  severity: 'low' | 'medium' | 'high'
}

export interface FraudSummary {
  total_clicks: number
  bot_clicks: number
  suspicious_clicks: number
  human_clicks: number
  invalid_rate: number
  datacenter_clicks: number
  total_spend: number
  wasted_spend: number
  real_cpc: number | null
  real_cpa: number | null
  conversions: number
  conversion_event: string
  signals: FraudSignal[]
}

export interface SourceQuality {
  source: string
  medium: string
  quality_score: number
  clicks: number
  invalid_clicks: number
  human_rate: number
}

export interface AdFraudCampaign {
  id: string
  name: string
  utm_source?: string
  utm_medium?: string
  utm_campaign?: string
  cpc: number
  cpm: number
  budget: number
  created_at: number
}

export interface AnalyticsFilters {
  country?: string
  browser?: string
  device?: string
  page?: string
  referrer?: string
  bot_filter?: string
}

export interface Domain {
  id: string
  name: string
  domain: string
  site_id: string
  is_active: boolean
  created_at: number
}

export interface License {
  tier: 'community' | 'pro' | 'enterprise'
  state: 'valid' | 'expired' | 'tampered' | 'missing'
  features: Record<string, boolean>
  limits: Record<string, number>
  expires_at: string | null
  licensee: string
}

export const defaultLicense: License = {
  tier: 'community',
  state: 'missing',
  features: {},
  limits: { max_users: 3, max_retention_days: 180 },
  expires_at: null,
  licensee: '',
}

// Consent Management types
export type ConsentCategoryId = 'necessary' | 'analytics' | 'marketing' | 'preferences' | string

export interface ConsentCategory {
  id: ConsentCategoryId
  label: string
  description: string
  required: boolean
  default_enabled: boolean
}

export interface ConsentAppearance {
  style: 'bar' | 'popup' | 'modal'
  position: 'top' | 'bottom' | 'bottom-left' | 'bottom-right' | 'center'
  bg_color: string
  text_color: string
  btn_bg_color: string
  btn_text_color: string
  show_reject_all: boolean
}

export interface ConsentConfig {
  id: string
  domain_id: string
  version: number
  is_active: boolean
  categories: ConsentCategory[]
  appearance: ConsentAppearance
  translations: Record<string, Record<string, string>>
  cookie_name: string
  cookie_expiry_days: number
  auto_language: boolean
  geo_targeting: string[]
  created_at: number
  updated_at: number
}

export interface ConsentRecord {
  id: string
  domain_id: string
  visitor_hash: string
  categories: Record<string, boolean>
  config_version: number
  action: 'show' | 'accept_all' | 'reject_all' | 'custom'
  user_agent: string
  geo_country: string
  timestamp: number
}

export interface ConsentTimeseriesPoint {
  period: string
  shows: number
  accept_all: number
  reject_all: number
  custom: number
}

export interface ConsentGeoBreakdown {
  country: string
  shows: number
  responses: number
  accept_all: number
  reject_all: number
  custom: number
  consent_rate: number
}

export interface ConsentAnalytics {
  shows: number
  total_responses: number
  accept_all_count: number
  reject_all_count: number
  custom_count: number
  consent_rate: number
  response_rate: number
  timeseries: ConsentTimeseriesPoint[]
  geo_breakdown: ConsentGeoBreakdown[]
}

// Privacy / GDPR types
export interface PrivacyAuditCheck {
  id: string
  category: string
  name: string
  description: string
  status: 'pass' | 'warn' | 'info'
  detail: string
}

export interface CookieInventoryItem {
  name: string
  purpose: string
  type: string
  set_by: string
  visitor_facing: boolean
  duration: string
  description: string
}

export interface DataInventoryItem {
  field: string
  purpose: string
  pii: string
  note: string
}

export interface DomainConsentStatus {
  domain_id: string
  domain_name: string
  domain: string
  has_consent: boolean
  version: number
}

export interface PrivacyAudit {
  checks: PrivacyAuditCheck[]
  cookie_inventory: CookieInventoryItem[]
  data_inventory: DataInventoryItem[]
  domain_consents: DomainConsentStatus[]
  storage_summary: Record<string, number>
  data_retention_days: number
  generated_at: string
}

export interface VisitorLookupResult {
  visitor_hash: string
  tables: Record<string, number>
  total_records: number
}

export interface ErasureResult {
  visitor_hash: string
  deleted: Record<string, number>
  total_deleted: number
  erased_at: string
}

// Audit Log types
export interface AuditLogEntry {
  id: string
  timestamp: number
  user_id: string
  user_email: string
  action: string
  resource_type: string
  resource_id: string
  detail: string
  ip_address: string
}

export interface AuditLogResponse {
  entries: AuditLogEntry[]
  total: number
  page: number
  per_page: number
}

// Tag Manager types
export type TagType = 'etiquetta_event' | 'custom_html' | 'ga4' | 'meta_pixel' | 'google_ads' | 'linkedin' | 'tiktok'
export type TriggerType = 'page_load' | 'dom_ready' | 'click_all' | 'click_specific' | 'scroll_depth' | 'custom_event' | 'timer' | 'history_change' | 'form_submit' | 'element_visibility'

export type SelectorMatchType = 'css' | 'id' | 'text' | 'data_attr' | 'link_url'
export type VariableType = 'data_layer' | 'url_param' | 'cookie' | 'dom_element' | 'js_variable' | 'constant' | 'referrer' | 'page_url' | 'page_path' | 'page_hostname'

export interface TMContainer {
  id: string
  domain_id: string
  name: string
  domain_name?: string
  domain?: string
  published_version: number
  draft_version: number
  published_at: number | null
  published_by: string | null
  created_at: number
  updated_at: number
}

export interface TMTag {
  id: string
  container_id: string
  name: string
  tag_type: TagType
  config: Record<string, unknown>
  consent_category: ConsentCategoryId
  priority: number
  is_enabled: boolean
  trigger_ids: string[]
  exception_trigger_ids: string[]
  version: number
  created_at: number
  updated_at: number
}

export interface TriggerCondition {
  variable: string
  operator: 'equals' | 'not_equals' | 'contains' | 'not_contains' | 'starts_with' | 'ends_with' | 'matches_regex'
  value: string
}

export interface TMTrigger {
  id: string
  container_id: string
  name: string
  trigger_type: TriggerType
  config: Record<string, unknown>
  created_at: number
  updated_at: number
}

export interface TMVariable {
  id: string
  container_id: string
  name: string
  variable_type: VariableType
  config: Record<string, unknown>
  created_at: number
  updated_at: number
}

// Period Comparison types
export interface ComparisonPeriod {
  start: number
  end: number
}

export interface ComparisonInsight {
  type: 'positive' | 'negative' | 'neutral'
  metric: string
  text: string
}

export interface ComparisonTimeseriesPoint {
  day_index: number
  date: string
  pageviews: number
  visitors: number
}

export interface ComparisonOverview {
  total_events: number
  unique_visitors: number
  sessions: number
  pageviews: number
  bounce_rate: number
  avg_session_seconds: number
}

export interface ComparisonResponse {
  current_period: ComparisonPeriod
  compare_period: ComparisonPeriod
  overview: { current: ComparisonOverview; previous: ComparisonOverview }
  timeseries: { current: ComparisonTimeseriesPoint[]; previous: ComparisonTimeseriesPoint[] }
  pages: { current: TopPage[]; previous: TopPage[] }
  referrers: { current: Referrer[]; previous: Referrer[] }
  geo: { current: GeoData[]; previous: GeoData[] }
  devices: { current: DeviceData[]; previous: DeviceData[] }
  browsers: { current: BrowserData[]; previous: BrowserData[] }
  campaigns: { current: Campaign[]; previous: Campaign[] }
  events: { current: CustomEvent[]; previous: CustomEvent[] }
  outbound: { current: OutboundLink[]; previous: OutboundLink[] }
  insights: ComparisonInsight[]
}

export interface TMSnapshot {
  id: string
  container_id: string
  version: number
  published_by: string
  published_at: number
}

// Ad Platform Connections types
export interface AdConnection {
  id: string
  provider: string
  name: string
  account_id: string
  status: 'active' | 'pending' | 'error' | 'disconnected'
  last_sync_at: number | null
  last_error: string | null
  config: Record<string, string>
  created_by: string
  created_at: number
  updated_at: number
}

export interface AdProvider {
  name: string
  display_name: string
  available: boolean
}

export interface AdSpendPoint {
  date: string
  provider: string
  cost: number
  impressions: number
  clicks: number
}

export interface AdAttributionRow {
  campaign: string
  source: string
  medium: string
  visits: number
  visitors: number
  sessions: number
  cost: number
  impressions: number
  ad_clicks: number
  cpc: number
  cpa: number
}

// Migration types
export type MigrateSource = 'ga4_bigquery' | 'ga4_csv' | 'plausible' | 'matomo' | 'umami' | 'csv' | 'gtm'
export type MigrateJobStatus = 'pending' | 'running' | 'completed' | 'failed' | 'cancelled' | 'rolled_back'

export interface ColumnMapping {
  source_column: string
  target_field: string | null
  auto_detected: boolean
  sample_values: string[]
}

export interface MigrateAnalysis {
  analysis_id: string
  source: MigrateSource
  columns: string[]
  sample_rows: string[][]
  row_estimate: number
  date_range: { start: string; end: string } | null
  suggested_mapping: Record<string, string>
  warnings: string[]
}

export interface MigrateJob {
  id: string
  source: MigrateSource
  domain: string
  status: MigrateJobStatus
  file_name: string
  file_size: number
  rows_total: number
  rows_imported: number
  rows_skipped: number
  error_message: string | null
  warnings: string[]
  started_at: number | null
  completed_at: number | null
  created_at: number
}

export interface GtmConvertResult {
  container: { tags: number; triggers: number; variables: number }
  warnings: string[]
  container_data: unknown
}

// Session Replay types
export interface SessionRecording {
  session_id: string
  domain: string
  visitor_hash: string
  start_time: number
  duration: number
  pages: number
  first_url: string
  device_type: string
  browser_name: string
  os_name: string
  geo_country: string
  screen_width: number
  screen_height: number
  size_bytes: number
  events_count: number
  status: string
  created_at: number
}

export interface ReplayListResponse {
  recordings: SessionRecording[]
  total: number
  limit: number
  offset: number
}

export interface ReplayData {
  session_id: string
  events: unknown[]
  metadata: SessionRecording
}

export interface SessionEvent {
  id: string
  timestamp: number
  event_type: string
  event_name: string
  url: string
  path: string
  page_title: string
  referrer_url: string
  utm_source: string
  utm_medium: string
  utm_campaign: string
  props: string
}

export interface ReplayStats {
  total_recordings: number
  total_size_bytes: number
  disk_usage_bytes: number
  quota_bytes: number
  quota_mb: number
}

export interface ReplaySettings {
  enabled: boolean
  sample_rate: number
  mask_text: boolean
  mask_inputs: boolean
  max_duration_sec: number
  storage_quota_mb: number
}

// Annotation types
export type AnnotationCategory = 'deployment' | 'analytics' | 'performance' | 'consent' | 'ads' | 'content' | 'other'

export interface Annotation {
  id: string
  domain_id: string
  date: string
  title: string
  description: string
  category: AnnotationCategory
  source: 'manual' | 'automatic'
  created_by: string
  created_at: number
  updated_at: number
}

export const ANNOTATION_CATEGORIES: Record<AnnotationCategory, { label: string; color: string }> = {
  deployment:  { label: 'Deployment',  color: '#3b82f6' },
  analytics:   { label: 'Analytics',   color: '#8b5cf6' },
  performance: { label: 'Performance', color: '#f59e0b' },
  consent:     { label: 'Consent',     color: '#10b981' },
  ads:         { label: 'Ads',         color: '#ef4444' },
  content:     { label: 'Content',     color: '#06b6d4' },
  other:       { label: 'Other',       color: '#6b7280' },
}
