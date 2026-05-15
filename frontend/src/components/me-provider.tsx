import { createContext, useContext, type ReactNode } from 'react'
import { useMe, type Me } from '@/hooks/use-me'

const MeContext = createContext<Me | null>(null)

export function useAuthedMe(): Me {
  const me = useContext(MeContext)
  if (!me) throw new Error('useAuthedMe must be used inside MeProvider')
  return me
}

// MeProvider loads /api/me and exposes the result via context. The Go
// middleware already gates the page server-side, so by the time we mount
// here we expect to be authed. The 'anon' branch only fires if the session
// died between the server gate and the /api/me fetch — defensively reload.
export function MeProvider({ children }: { children: ReactNode }) {
  const state = useMe()

  if (state.kind === 'loading') return null
  if (state.kind === 'anon') {
    window.location.assign('/login')
    return null
  }

  return <MeContext.Provider value={state.me}>{children}</MeContext.Provider>
}
