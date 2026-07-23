# @etiquetta/next

Next.js integration for [Etiquetta](https://etiquetta.com) analytics.

## Install

```bash
bun add @etiquetta/next
# or
npm install @etiquetta/next
```

## Usage

### Browser tracking (App Router)

Drop the script tag into your root layout:

```tsx
// app/layout.tsx
import { EtiquettaScript } from '@etiquetta/next/script'

export default function RootLayout({ children }) {
  return (
    <html lang="en">
      <head>
        <EtiquettaScript endpoint={process.env.NEXT_PUBLIC_ETIQUETTA_URL!} siteId={process.env.NEXT_PUBLIC_ETIQUETTA_SITE_ID!} />
      </head>
      <body>{children}</body>
    </html>
  )
}
```

### Server-side custom events

```ts
// app/actions.ts
'use server'
import { configure, trackServer } from '@etiquetta/next'

configure({ endpoint: process.env.ETIQUETTA_URL!, siteId: process.env.ETIQUETTA_SITE_ID! })

export async function signupAction(data: FormData) {
  // ... create the user ...
  trackServer({
    type: 'custom',
    event: 'signup_completed',
    visitorId: user.id,
    properties: { plan: 'pro' },
  })
}
```

### Edge middleware (automatic pageviews without a browser script)

```ts
// middleware.ts
import { NextResponse } from 'next/server'
import type { NextRequest, NextFetchEvent } from 'next/server'
import { configure, etiquettaMiddleware } from '@etiquetta/next/middleware'

configure({ endpoint: process.env.ETIQUETTA_URL!, siteId: process.env.ETIQUETTA_SITE_ID! })

export function middleware(req: NextRequest, event: NextFetchEvent) {
  etiquettaMiddleware(req, event)
  return NextResponse.next()
}

export const config = {
  matcher: ['/((?!api|_next/static|_next/image|favicon.ico).*)'],
}
```

Edge middleware gives you cookie-less, script-less pageview tracking that works even for users with JS disabled — at the cost of not capturing client-side navigations in SPAs.

## Exports

| Module | What it exports |
| ------ | --------------- |
| `@etiquetta/next` | `configure`, `trackServer`, `flushServer`, plus re-exports from `@etiquetta/node` |
| `@etiquetta/next/script` | `<EtiquettaScript>` React component |
| `@etiquetta/next/middleware` | `configure`, `etiquettaMiddleware` for Edge middleware |
