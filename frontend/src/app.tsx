import type { ComponentType } from 'react';
import { lazy, Suspense } from 'react';
import { Route, Router, Switch } from 'wouter';

import { Toaster } from '@/components/ui/sonner';
import { useAppLocation } from '@/hooks/use-app-location';
import { AppShell } from '@/layouts/app-shell';
import { PATHS } from '@/lib/paths';
import { Digest } from '@/pages/digest';
import { Login } from '@/pages/login';

import { Entry } from '@/pages/entry';
import { Library } from '@/pages/library';
import { NewsletterSourcePage as NewsletterSource } from '@/pages/newsletter-source';
import { Source } from '@/pages/source';
import { Sources } from '@/pages/sources';
import { Settings } from '@/pages/settings';
import { ImportSources } from '@/pages/import-sources';
import { ExportSources } from '@/pages/export-sources';
const DevRoutes = import.meta.env.DEV
    ? lazy(() => import('@/dev/routes').then((m) => ({ default: m.DevRoutes })))
    : null;

type PageDef = { path: string; Component: ComponentType };

const CHROME_PAGES: PageDef[] = [
    { path: PATHS.library, Component: Library },
    { path: PATHS.sources, Component: Sources },
    { path: PATHS.settings, Component: Settings },
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
                    <Route path="/dev/styleguide">
                        <Suspense fallback={null}>
                            <DevRoutes />
                        </Suspense>
                    </Route>
                ) : null}
                <Route path={PATHS.digest}>
                    <AppShell>
                        <Digest />
                    </AppShell>
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
            <Toaster />
        </Router>
    );
}
