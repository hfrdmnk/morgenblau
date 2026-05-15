import { useEffect, useState } from 'react'
import { useDocumentTitle } from '@/hooks/use-document-title'

type Subscription = {
  uri: string
  cid: string
  value: {
    title?: string
    feedUrl?: string
    siteUrl?: string
    [k: string]: unknown
  }
}

type State =
  | { kind: 'loading' }
  | { kind: 'ok'; records: Subscription[] }
  | { kind: 'error' }

export function Sources() {
  useDocumentTitle('Sources')
  const [state, setState] = useState<State>({ kind: 'loading' })

  useEffect(() => {
    let cancelled = false
    fetch('/api/subscriptions')
      .then((r) => {
        if (!r.ok) throw new Error(String(r.status))
        return r.json()
      })
      .then((records: Subscription[]) => {
        if (!cancelled) setState({ kind: 'ok', records })
      })
      .catch(() => {
        if (!cancelled) setState({ kind: 'error' })
      })
    return () => { cancelled = true }
  }, [])

  if (state.kind === 'loading') {
    return <main className="p-8"><p className="text-gray-700">Loading…</p></main>
  }
  if (state.kind === 'error') {
    return <main className="p-8"><p className="text-gray-700">Could not load your subscriptions.</p></main>
  }
  if (state.records.length === 0) {
    return <main className="p-8"><p className="text-gray-700">No subscriptions yet.</p></main>
  }
  return (
    <main className="p-8">
      <ul className="space-y-2">
        {state.records.map((r) => (
          <li key={r.uri}>
            {r.value.siteUrl ? (
              <a href={r.value.siteUrl} className="underline">
                {r.value.title || r.value.feedUrl || r.uri}
              </a>
            ) : (
              <span>{r.value.title || r.value.feedUrl || r.uri}</span>
            )}
          </li>
        ))}
      </ul>
    </main>
  )
}
