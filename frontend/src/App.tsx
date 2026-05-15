import type { FC, ReactNode } from 'react'
import { MeProvider } from '@/components/me-provider'
import WindowLayout from '@/layouts/window-layout'
import { Welcome } from '@/pages/Welcome'
import { Login } from '@/pages/Login'
import { Consume } from '@/pages/Consume'
import { Sources } from '@/pages/Sources'
import { Entry } from '@/pages/Entry'

// Path → component. Auth and authed-redirects are decided by the Go
// middleware (see internal/routes/routes.json); the client only maps paths
// to what gets rendered.
type PageDef = { Component: FC; authed: boolean; chrome: boolean }

const pages: Record<string, PageDef> = {
  '/': { Component: Welcome, authed: false, chrome: false },
  '/login': { Component: Login, authed: false, chrome: false },
  '/consume': { Component: Consume, authed: true, chrome: true },
  '/sources': { Component: Sources, authed: true, chrome: true },
  '/entry': { Component: Entry, authed: true, chrome: false },
}

export default function App() {
  const def = pages[window.location.pathname]
  if (!def) return null
  const { Component, authed, chrome } = def
  const inner: ReactNode = chrome ? (
    <WindowLayout>
      <Component />
    </WindowLayout>
  ) : (
    <Component />
  )
  return authed ? <MeProvider>{inner}</MeProvider> : inner
}
