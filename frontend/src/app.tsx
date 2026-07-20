import type { ComponentType } from 'react';
import { lazy, Suspense, useState } from 'react';
import { Route, Router, Switch } from 'wouter';

import { KeyboardHelp } from '@/components/keyboard-help';
import { MeProvider } from '@/components/me-provider';
import { RouteSkeleton } from '@/components/route-skeleton';
import { Toaster } from '@/components/ui/sonner';
import { useAppLocation } from '@/hooks/use-app-location';
import { AppLayout } from '@/layouts/app-layout';
import { PATHS } from '@/lib/paths';

// Named exports, not default, so each lazy() adapts the module to the shape React.lazy requires.
const Digest = lazy(() => import('@/pages/digest').then((m) => ({ default: m.Digest })));
const Discover = lazy(() => import('@/pages/discover').then((m) => ({ default: m.Discover })));
const Entry = lazy(() => import('@/pages/entry').then((m) => ({ default: m.Entry })));
const Library = lazy(() => import('@/pages/library').then((m) => ({ default: m.Library })));
const Login = lazy(() => import('@/pages/login').then((m) => ({ default: m.Login })));
const Profile = lazy(() => import('@/pages/profile').then((m) => ({ default: m.Profile })));
const Source = lazy(() => import('@/pages/source').then((m) => ({ default: m.Source })));
const Sources = lazy(() => import('@/pages/sources').then((m) => ({ default: m.Sources })));
const Welcome = lazy(() => import('@/pages/welcome').then((m) => ({ default: m.Welcome })));
const DEV_STYLEGUIDE_PATH = '/styleguide';
const Styleguide = import.meta.env.DEV
    ? lazy(() =>
          import('@/pages/styleguide').then((m) => ({
              default: m.Styleguide,
          })),
      )
    : null;

type PageDef = { path: string; Component: ComponentType };

// Nested under one stable Route: Switch keys output on the matched element, so flat per-page Routes would remount AppLayout on every tab switch.
const CHROME_PAGES: PageDef[] = [
    { path: PATHS.digest, Component: Digest },
    { path: PATHS.library, Component: Library },
    { path: PATHS.sources, Component: Sources },
    { path: `${PATHS.sources}/:rkey`, Component: Source },
    { path: PATHS.discover, Component: Discover },
    { path: `${PATHS.profile}/:handleOrDid`, Component: Profile },
];

// Derived from CHROME_PAGES so the outer Route can't silently drift from the inner Switch.
const CHROME_PATTERN = new RegExp(
    `^(?:${CHROME_PAGES.map(({ path }) =>
        path.replace(/[.*+?^${}()|[\]\\]/g, '\\$&').replace(/:[^/]+/g, '[^/]+'),
    ).join('|')})$`,
);

// Auth transitions are server redirects, so one check per mount suffices before wrapping authed routes in MeProvider.
function isAuthedPath(pathname: string): boolean {
    const isDevStyleguide =
        import.meta.env.DEV && pathname === DEV_STYLEGUIDE_PATH;
    return (
        pathname !== PATHS.welcome &&
        pathname !== PATHS.login &&
        !isDevStyleguide
    );
}

export default function App() {
    const [authed] = useState(() => isAuthedPath(window.location.pathname));

    const router = (
        <Router hook={useAppLocation}>
            <Suspense fallback={<RouteSkeleton />}>
                <Switch>
                    <Route path={PATHS.welcome}>
                        <Welcome />
                    </Route>
                    <Route path={PATHS.login}>
                        <Login />
                    </Route>
                    <Route path={`${PATHS.entry}/:slug`}>
                        <Entry />
                    </Route>
                    {Styleguide ? (
                        <Route path={DEV_STYLEGUIDE_PATH}>
                            <Styleguide />
                        </Route>
                    ) : null}
                    <Route path={CHROME_PATTERN}>
                        <AppLayout>
                            {/* Nested so a page-chunk suspense doesn't bubble past AppLayout and remount the chrome. */}
                            <Suspense fallback={<RouteSkeleton />}>
                                <Switch>
                                    {CHROME_PAGES.map(({ path, Component }) => (
                                        <Route key={path} path={path}>
                                            <Component />
                                        </Route>
                                    ))}
                                </Switch>
                            </Suspense>
                        </AppLayout>
                    </Route>
                </Switch>
            </Suspense>
        </Router>
    );

    return authed ? (
        <MeProvider>
            {router}
            <KeyboardHelp />
            <Toaster />
        </MeProvider>
    ) : (
        router
    );
}
