import type { FC, ReactNode } from 'react';

import { MeProvider } from '@/components/me-provider';
import { WindowLayout } from '@/layouts/window-layout';
import { PATHS } from '@/lib/paths';
import { Consume } from '@/pages/consume';
import { Create } from '@/pages/create';
import { Discover } from '@/pages/discover';
import { Entry } from '@/pages/entry';
import { Login } from '@/pages/login';
import { Sources } from '@/pages/sources';
import { Welcome } from '@/pages/welcome';

// Path → component. Auth and authed-redirects are decided by the Go
// middleware (see internal/routes/routes.json); the client only maps paths
// to what gets rendered.
type PageDef = { Component: FC; authed: boolean; chrome: boolean };

const pages: Record<string, PageDef> = {
    [PATHS.welcome]: { Component: Welcome, authed: false, chrome: false },
    [PATHS.login]: { Component: Login, authed: false, chrome: false },
    [PATHS.consume]: { Component: Consume, authed: true, chrome: true },
    [PATHS.sources]: { Component: Sources, authed: true, chrome: true },
    [PATHS.discover]: { Component: Discover, authed: true, chrome: true },
    [PATHS.create]: { Component: Create, authed: true, chrome: true },
    // TODO: switch to /entry/:slug when real data lands.
    [PATHS.entry]: { Component: Entry, authed: true, chrome: false },
};

export default function App() {
    const def = pages[window.location.pathname];
    if (!def) return null;
    const { Component, authed, chrome } = def;
    const inner: ReactNode = chrome ? (
        <WindowLayout>
            <Component />
        </WindowLayout>
    ) : (
        <Component />
    );
    return authed ? <MeProvider>{inner}</MeProvider> : inner;
}
