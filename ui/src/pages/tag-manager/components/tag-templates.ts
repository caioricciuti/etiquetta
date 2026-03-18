import type { TriggerCondition } from '@/lib/types'

export interface TagConfigField {
  key: string
  label: string
  type: 'text' | 'textarea' | 'select'
  placeholder?: string
  required?: boolean
  options?: { label: string; value: string }[]
}

export interface TagPrivacyMeta {
  externalDomains: string[]
  setsCookies: boolean
  privacyRisk: 'low' | 'medium' | 'high'
  privacyNote: string
}

export interface TagTemplate {
  type: string
  name: string
  description: string
  icon: string
  configFields: TagConfigField[]
  privacy: TagPrivacyMeta
}

export const TAG_TEMPLATES: TagTemplate[] = [
  {
    type: 'etiquetta_event',
    name: 'Etiquetta Event',
    description: 'Send a custom event to Etiquetta analytics',
    icon: 'Activity',
    configFields: [
      { key: 'event_name', label: 'Event Name', type: 'text', required: true, placeholder: 'e.g., purchase, signup, download' },
      { key: 'event_props', label: 'Event Properties (JSON)', type: 'textarea', placeholder: '{"plan": "pro", "value": 99}' },
    ],
    privacy: {
      externalDomains: [],
      setsCookies: false,
      privacyRisk: 'low',
      privacyNote: 'First-party only. No data leaves your server.',
    },
  },
  {
    type: 'custom_html',
    name: 'Custom HTML',
    description: 'Inject custom HTML/JavaScript',
    icon: 'Code',
    configFields: [
      { key: 'html', label: 'HTML Code', type: 'textarea', required: true, placeholder: '<script>...</script>' },
    ],
    privacy: {
      externalDomains: [],
      setsCookies: false,
      privacyRisk: 'high',
      privacyNote: 'Arbitrary code — cannot be audited automatically. May load external resources or set cookies.',
    },
  },
  {
    type: 'ga4',
    name: 'Google Analytics 4',
    description: 'Google Analytics measurement',
    icon: 'BarChart',
    configFields: [
      { key: 'measurement_id', label: 'Measurement ID', type: 'text', required: true, placeholder: 'G-XXXXXXXXXX' },
    ],
    privacy: {
      externalDomains: ['www.googletagmanager.com', 'www.google-analytics.com'],
      setsCookies: true,
      privacyRisk: 'high',
      privacyNote: 'Sends visitor data to Google. Sets _ga cookies. Requires marketing/analytics consent.',
    },
  },
  {
    type: 'meta_pixel',
    name: 'Meta Pixel',
    description: 'Facebook/Meta tracking pixel',
    icon: 'Facebook',
    configFields: [
      { key: 'pixel_id', label: 'Pixel ID', type: 'text', required: true, placeholder: '123456789' },
    ],
    privacy: {
      externalDomains: ['connect.facebook.net'],
      setsCookies: true,
      privacyRisk: 'high',
      privacyNote: 'Sends visitor data to Meta. Sets _fbp cookies. Cross-site tracking.',
    },
  },
  {
    type: 'google_ads',
    name: 'Google Ads',
    description: 'Google Ads conversion tracking',
    icon: 'Target',
    configFields: [
      { key: 'conversion_id', label: 'Conversion ID', type: 'text', required: true, placeholder: 'AW-XXXXXXXXX' },
      { key: 'conversion_label', label: 'Conversion Label', type: 'text', placeholder: 'optional' },
    ],
    privacy: {
      externalDomains: ['www.googletagmanager.com', 'www.googleadservices.com'],
      setsCookies: true,
      privacyRisk: 'high',
      privacyNote: 'Sends conversion data to Google Ads. Sets cookies for attribution.',
    },
  },
  {
    type: 'linkedin',
    name: 'LinkedIn Insight',
    description: 'LinkedIn conversion tracking',
    icon: 'Linkedin',
    configFields: [
      { key: 'partner_id', label: 'Partner ID', type: 'text', required: true },
    ],
    privacy: {
      externalDomains: ['snap.licdn.com'],
      setsCookies: true,
      privacyRisk: 'medium',
      privacyNote: 'Sends page view data to LinkedIn for ad targeting.',
    },
  },
  {
    type: 'tiktok',
    name: 'TikTok Pixel',
    description: 'TikTok tracking pixel',
    icon: 'Music',
    configFields: [
      { key: 'pixel_id', label: 'Pixel ID', type: 'text', required: true },
    ],
    privacy: {
      externalDomains: ['analytics.tiktok.com'],
      setsCookies: true,
      privacyRisk: 'high',
      privacyNote: 'Sends visitor data to TikTok. Cross-site tracking for ad attribution.',
    },
  },
]

export function getTemplate(type: string): TagTemplate | undefined {
  return TAG_TEMPLATES.find((t) => t.type === type)
}

export const TRIGGER_TYPE_LABELS: Record<string, string> = {
  page_load: 'Page Load',
  dom_ready: 'DOM Ready',
  click_all: 'All Clicks',
  click_specific: 'Specific Click',
  scroll_depth: 'Scroll Depth',
  custom_event: 'Custom Event',
  timer: 'Timer',
  history_change: 'History Change',
  form_submit: 'Form Submit',
  element_visibility: 'Element Visibility',
}

export const TRIGGER_TYPE_DESCRIPTIONS: Record<string, string> = {
  page_load: 'Fires when the page starts loading',
  dom_ready: 'Fires when the DOM is fully parsed',
  click_all: 'Fires on any click anywhere on the page',
  click_specific: 'Fires when a specific element is clicked',
  scroll_depth: 'Fires when the user scrolls past a threshold',
  custom_event: 'Fires when a custom JavaScript event is dispatched',
  timer: 'Fires on a timed interval',
  history_change: 'Fires on SPA navigation (pushState/popstate)',
  form_submit: 'Fires when a form is submitted',
  element_visibility: 'Fires when a specific element becomes visible in the viewport',
}

export const TRIGGER_TYPE_ICONS: Record<string, string> = {
  page_load: 'FileText',
  dom_ready: 'LayoutTemplate',
  click_all: 'MousePointer',
  click_specific: 'MousePointerClick',
  scroll_depth: 'ArrowDownToLine',
  custom_event: 'Sparkles',
  timer: 'Clock',
  history_change: 'Route',
  form_submit: 'FormInput',
  element_visibility: 'Eye',
}

// Trigger types that need a selector (element targeting)
export const SELECTOR_TRIGGER_TYPES = new Set(['click_specific', 'form_submit', 'element_visibility'])

// Match types for element selection
export const SELECTOR_MATCH_TYPES = [
  { value: 'css', label: 'CSS Selector', description: 'Match using any CSS selector', placeholder: '.btn-primary, #checkout', icon: 'Code' },
  { value: 'id', label: 'Element ID', description: 'Match by the element\'s ID attribute', placeholder: 'buy-button', icon: 'Hash' },
  { value: 'data_attr', label: 'Data Attribute', description: 'Match by a data-* attribute', placeholder: '', icon: 'Database' },
  { value: 'text', label: 'Text Content', description: 'Match elements containing specific text', placeholder: 'Add to Cart', icon: 'Type' },
  { value: 'link_url', label: 'Link URL', description: 'Match links by their href', placeholder: '/checkout', icon: 'Link' },
] as const

export const VARIABLE_TYPE_LABELS: Record<string, string> = {
  data_layer: 'Data Layer',
  url_param: 'URL Parameter',
  cookie: 'Cookie',
  dom_element: 'DOM Element',
  js_variable: 'JavaScript Variable',
  constant: 'Constant',
  referrer: 'Referrer',
  page_url: 'Page URL',
  page_path: 'Page Path',
  page_hostname: 'Page Hostname',
}

export const CONSENT_CATEGORIES = [
  { label: 'Necessary', value: 'necessary' },
  { label: 'Analytics', value: 'analytics' },
  { label: 'Marketing', value: 'marketing' },
  { label: 'Preferences', value: 'preferences' },
]

export interface TriggerConfigField {
  key: string
  label: string
  type: 'text' | 'number'
  placeholder?: string
  required?: boolean
}

export const TRIGGER_CONFIG_FIELDS: Record<string, TriggerConfigField[]> = {
  custom_event: [
    { key: 'event_name', label: 'Event Name', type: 'text', required: true, placeholder: 'e.g., purchase' },
  ],
  scroll_depth: [
    { key: 'percentage', label: 'Scroll Percentage', type: 'number', required: true, placeholder: '50' },
  ],
  timer: [
    { key: 'interval_ms', label: 'Interval (ms)', type: 'number', required: true, placeholder: '5000' },
    { key: 'limit', label: 'Max Fires', type: 'number', placeholder: '1' },
  ],
  element_visibility: [
    { key: 'threshold', label: 'Visibility Threshold (%)', type: 'number', required: true, placeholder: '50' },
    { key: 'fire_once', label: 'Fire Once', type: 'text', placeholder: 'true' },
  ],
  // click_specific, form_submit, element_visibility selectors handled by SelectorBuilder
}

export interface VariableConfigField {
  key: string
  label: string
  type: 'text'
  placeholder?: string
  required?: boolean
}

export const VARIABLE_CONFIG_FIELDS: Record<string, VariableConfigField[]> = {
  data_layer: [
    { key: 'variable_name', label: 'Variable Name', type: 'text', required: true, placeholder: 'ecommerce.purchase.revenue' },
  ],
  url_param: [
    { key: 'param_name', label: 'Parameter Name', type: 'text', required: true, placeholder: 'utm_source' },
  ],
  cookie: [
    { key: 'cookie_name', label: 'Cookie Name', type: 'text', required: true, placeholder: '_ga' },
  ],
  dom_element: [
    { key: 'selector', label: 'CSS Selector', type: 'text', required: true, placeholder: '#price' },
    { key: 'attribute', label: 'Attribute (optional)', type: 'text', placeholder: 'data-value' },
  ],
  js_variable: [
    { key: 'variable_name', label: 'Global Variable', type: 'text', required: true, placeholder: 'window.userId' },
  ],
  constant: [
    { key: 'value', label: 'Value', type: 'text', required: true, placeholder: 'my-constant-value' },
  ],
}

// Condition operators for trigger conditions (Feature 2)
export const CONDITION_OPERATORS: { label: string; value: TriggerCondition['operator'] }[] = [
  { label: 'equals', value: 'equals' },
  { label: 'does not equal', value: 'not_equals' },
  { label: 'contains', value: 'contains' },
  { label: 'does not contain', value: 'not_contains' },
  { label: 'starts with', value: 'starts_with' },
  { label: 'ends with', value: 'ends_with' },
  { label: 'matches regex', value: 'matches_regex' },
]

// Built-in variables available for trigger conditions
export const BUILT_IN_CONDITION_VARIABLES: { label: string; value: string }[] = [
  { label: 'Page Path', value: 'page_path' },
  { label: 'Page URL', value: 'page_url' },
  { label: 'Page Hostname', value: 'page_hostname' },
  { label: 'Referrer', value: 'referrer' },
]
