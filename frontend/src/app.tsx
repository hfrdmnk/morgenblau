import type { ComponentType } from 'react';
import { lazy, Suspense } from 'react';
import { Route, Router, Switch } from 'wouter';

import { Placeholder } from '@/components/placeholder';
import { useAppLocation } from '@/hooks/use-app-location';
import { PATHS } from '@/lib/paths';

const Digest = lazy(() => import('@/pages/digest').then((m) => ({ default: m.Digest })));
const Entry = lazy(() => import('@/pages/entry').then((m) => ({ default: m.Entry })));
const Library = lazy(() => import('@/pages/library').then((m) => ({ default: m.Library })));
const NewsletterSource = lazy(() =>
    import('@/pages/newsletter-source').then((m) => ({ default: m.NewsletterSourcePage })),
);
const Login = lazy(() => import('@/pages/login').then((m) => ({ default: m.Login })));
const Source = lazy(() => import('@/pages/source').then((m) => ({ default: m.Source })));
const Sources = lazy(() => import('@/pages/sources').then((m) => ({ default: m.Sources })));
const Settings = lazy(() => import('@/pages/settings').then((m) => ({ default: m.Settings })));
const Welcome = lazy(() => import('@/pages/welcome').then((m) => ({ default: m.Welcome })));
const DEV_STYLEGUIDE_PATH = '/styleguide';
const Styleguide = import.meta.env.DEV
    ? lazy(() => import('@/pages/styleguide').then((m) => ({ default: m.Styleguide })))
    : null;

type PageDef = { path: string; Component: ComponentType };

const CHROME_PAGES: PageDef[] = [
    { path: PATHS.digest, Component: Digest },
    { path: PATHS.library, Component: Library },
    { path: PATHS.sources, Component: Sources },
    { path: PATHS.settings, Component: Settings },
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
            <Suspense fallback={<Placeholder label="Loading" />}>
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
        </Router>
    );
}
