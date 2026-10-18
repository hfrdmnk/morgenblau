import type { ComponentType } from 'react';
import { lazy, Suspense } from 'react';
import { Route, Router, Switch } from 'wouter';

import { Toaster } from '@/components/ui/sonner';
import { useAppLocation } from '@/hooks/use-app-location';
import { AppShell } from '@/layouts/app-shell';
import { PATHS } from '@/lib/paths';
import { Admin } from '@/pages/admin';
import { Digest } from '@/pages/digest';
import { Entry } from '@/pages/entry';
import { ExportSources } from '@/pages/export-sources';
import { ImportSources } from '@/pages/import-sources';
import { Library } from '@/pages/library';
import { Login } from '@/pages/login';
import { NewsletterSource } from '@/pages/newsletter-source';
import { Settings } from '@/pages/settings';
import { Source } from '@/pages/source';
import { Sources } from '@/pages/sources';

const DevRoutes = import.meta.env.DEV
    ? lazy(() => import('@/dev/routes').then((m) => ({ default: m.DevRoutes })))
    : null;

type PageDef = { path: string; Component: ComponentType };

const CHROME_PAGES: PageDef[] = [
    { path: PATHS.library, Component: Library },
    { path: PATHS.admin, Component: Admin },
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
                    <Entry />
                </Route>
                {DevRoutes ? (
                    <Route path="/__dev/styleguide">
                        <Suspense fallback={null}>
                            <DevRoutes />
                        </Suspense>
                    </Route>
                ) : null}
                <Route>
                    <AppShell>
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
                    </AppShell>
                </Route>
            </Switch>
            <Toaster />
        </Router>
    );
}
