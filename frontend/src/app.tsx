import type { ComponentType } from 'react';
import { lazy, Suspense } from 'react';
import { Route, Router, Switch } from 'wouter';

import { Placeholder } from '@/components/placeholder';
import { Toaster } from '@/components/ui/sonner';
import { useAppLocation } from '@/hooks/use-app-location';
import { AppShell } from '@/layouts/app-shell';
import { PATHS } from '@/lib/paths';
import { Digest } from '@/pages/digest';
import { Login } from '@/pages/login';

const Entry = lazy(() => import('@/pages/entry').then((m) => ({ default: m.Entry })));
const Library = lazy(() => import('@/pages/library').then((m) => ({ default: m.Library })));
const NewsletterSource = lazy(() =>
    import('@/pages/newsletter-source').then((m) => ({ default: m.NewsletterSourcePage })),
);
const Source = lazy(() => import('@/pages/source').then((m) => ({ default: m.Source })));
const Sources = lazy(() => import('@/pages/sources').then((m) => ({ default: m.Sources })));
const Settings = lazy(() => import('@/pages/settings').then((m) => ({ default: m.Settings })));
const ImportSources = lazy(() =>
    import('@/pages/import-sources').then((m) => ({ default: m.ImportSources })),
);
const ExportSources = lazy(() =>
    import('@/pages/export-sources').then((m) => ({ default: m.ExportSources })),
);
const DevRoutes = import.meta.env.DEV
    ? lazy(() => import('@/dev/routes').then((m) => ({ default: m.DevRoutes })))
    : null;

type PageDef = { path: string; Component: ComponentType };

const CHROME_PAGES: PageDef[] = [
    { path: PATHS.library, Component: Library },
    { path: PATHS.sources, Component: Sources },
    { path: PATHS.settings, Component: Settings },
    { path: PATHS.general, Component: Settings },
    { path: PATHS.import, Component: ImportSources },
    { path: PATHS.export, Component: ExportSources },
    { path: `${PATHS.sources}/newsletters/:id`, Component: NewsletterSource },
    { path: `${PATHS.sources}/:rkey`, Component: Source },
];

const CHROME_PATTERN = new RegExp(
    `^(?:${CHROME_PAGES.map(({ path }) =>
        path.replace(/[.*+?^${}()|[\]\\]/g, '\\$&').replace(/:[^/]+/g, '[^/]+'),
    ).join('|')})$`,
);

export default function App() {
    return (
        <Router hook={useAppLocation}>
            <Switch>
                <Route path={PATHS.login}>
                    <Login />
                </Route>
                <Route path={`${PATHS.entry}/:slug`}>
                    <Suspense fallback={<Placeholder label="Loading" />}>
                        <Entry />
                    </Suspense>
                </Route>
                {DevRoutes ? (
                    <Route path="/dev/styleguide">
                        <Suspense fallback={<Placeholder label="Loading" />}>
                            <DevRoutes />
                        </Suspense>
                    </Route>
                ) : null}
                <Route>
                    <AppShell>
                        <Suspense fallback={<Placeholder label="Loading" />}>
                            <Switch>
                                <Route path={PATHS.digest}>
                                    <Digest />
                                </Route>
                                <Route path={CHROME_PATTERN}>
                                    <Switch>
                                        {CHROME_PAGES.map(({ path, Component }) => (
                                            <Route key={path} path={path}>
                                                <Component />
                                            </Route>
                                        ))}
                                    </Switch>
                                </Route>
                            </Switch>
                        </Suspense>
                    </AppShell>
                </Route>
            </Switch>
            <Toaster />
        </Router>
    );
}
