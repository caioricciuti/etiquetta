import type { MigrateSource } from '../../../lib/types'

export interface SourceConfig {
  id: MigrateSource
  name: string
  description: string
  accept: string
  instructions: string[]
}

export const sourceConfigs: SourceConfig[] = [
  {
    id: 'plausible',
    name: 'Plausible Analytics',
    description: 'Import page analytics data from Plausible CSV exports.',
    accept: '.csv',
    instructions: [
      'Go to your Plausible dashboard',
      'Navigate to Settings → Danger Zone',
      'Click "Export Data"',
      'Unzip the downloaded file',
      'Upload the pages CSV file here',
    ],
  },
  {
    id: 'ga4_csv',
    name: 'Google Analytics 4 (CSV)',
    description: 'Import aggregated report data exported from GA4 as CSV.',
    accept: '.csv',
    instructions: [
      'Go to Google Analytics → Reports',
      'Open the Pages report',
      'Set your desired date range',
      'Click the download icon → CSV',
      'Upload the downloaded CSV here',
    ],
  },
  {
    id: 'ga4_bigquery',
    name: 'Google Analytics 4 (BigQuery)',
    description: 'Import raw event-level data from GA4 BigQuery export (NDJSON).',
    accept: '.json,.ndjson,.jsonl',
    instructions: [
      'Open BigQuery in Google Cloud Console',
      'Run: SELECT * FROM `project.analytics_XXX.events_*`',
      'Export results as NDJSON (newline-delimited JSON)',
      'Upload the exported file here',
    ],
  },
  {
    id: 'matomo',
    name: 'Matomo',
    description: 'Import page analytics data from Matomo CSV exports.',
    accept: '.csv',
    instructions: [
      'Go to your Matomo dashboard',
      'Navigate to the Pages report',
      'Select your date range',
      'Click Export → CSV',
      'Upload the downloaded CSV here',
    ],
  },
  {
    id: 'umami',
    name: 'Umami',
    description: 'Import analytics data from Umami CSV exports.',
    accept: '.csv',
    instructions: [
      'Go to your Umami dashboard',
      'Navigate to Settings → Websites',
      'Click on your website → Data tab',
      'Click Export and select CSV',
      'Upload the downloaded CSV here',
    ],
  },
  {
    id: 'csv',
    name: 'Generic CSV',
    description: 'Import from any CSV file with custom column mapping.',
    accept: '.csv',
    instructions: [
      'Prepare a CSV file with headers',
      'Upload it here',
      'You\'ll map columns to Etiquetta fields in the next step',
    ],
  },
  {
    id: 'gtm',
    name: 'Google Tag Manager',
    description: 'Convert a GTM container export into an Etiquetta tag manager container.',
    accept: '.json',
    instructions: [
      'Go to Google Tag Manager',
      'Navigate to Admin → Export Container',
      'Download the JSON file',
      'Upload it here',
    ],
  },
]
