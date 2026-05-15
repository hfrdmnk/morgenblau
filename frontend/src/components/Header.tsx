import { useEffect, useState } from 'react'

type Me = { did: string; handle: string }

export function Header() {
  const [me, setMe] = useState<Me | null>(null)

  useEffect(() => {
    let cancelled = false
    fetch('/api/me')
      .then((r) => (r.ok ? r.json() : null))
      .then((data) => { if (!cancelled) setMe(data) })
      .catch(() => { if (!cancelled) setMe(null) })
    return () => { cancelled = true }
  }, [])

  return (
    <header className="flex items-center justify-between p-4 border-b">
      <span className="text-sm text-gray-700">
        {me ? `Signed in as @${me.handle}` : ' '}
      </span>
      <form method="POST" action="/oauth/logout">
        <button type="submit" className="text-sm text-gray-700 underline">
          Log out
        </button>
      </form>
    </header>
  )
}
