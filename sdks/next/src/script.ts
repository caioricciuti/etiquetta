/**
 * Browser tracker snippet for Next.js apps.
 *
 * Drop `<EtiquettaScript endpoint="..." siteId="..." />` into your root layout
 * to load the Etiquetta browser tracker.
 */

import React from 'react'

export interface EtiquettaScriptProps {
  /** Full origin of your Etiquetta server, e.g. https://analytics.example.com */
  endpoint: string
  /** Site ID from Etiquetta settings. */
  siteId: string
  /** Defer script loading. Defaults to true. */
  defer?: boolean
}

export function EtiquettaScript({ endpoint, siteId, defer = true }: EtiquettaScriptProps) {
  const src = `${endpoint.replace(/\/+$/, '')}/s.js`
  // The tracker reads its site ID from the `data-site` attribute and derives
  // the ingest origin from the script's own `src`.
  return React.createElement('script', {
    src,
    'data-site': siteId,
    defer,
    async: !defer,
  })
}
