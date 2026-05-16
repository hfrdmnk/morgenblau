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

type PageDef = { Component: FC; authed: boolean; chrome: boolean };

const pages: Record<string, PageDef> = {
    [PATHS.welcome]: { Component: Welcome, authed: false, chrome: false },
    [PATHS.login]: { Component: Login, authed: false, chrome: false },
    [PATHS.consume]: { Component: Consume, authed: true, chrome: true },
    [PATHS.sources]: { Component: Sources, authed: true, chrome: true },
    [PATHS.discover]: { Component: Discover, authed: true, chrome: true },
    [PATHS.create]: { Component: Create, authed: true, chrome: true },
};

const entryDef: PageDef = { Component: Entry, authed: true, chrome: false };

function resolvePage(pathname: string): PageDef | null {
    if (pages[pathname]) return pages[pathname];
    if (pathname.startsWith(`${PATHS.entry}/`)) return entryDef;
    return null;
}

export default function App() {
    const def = resolvePage(window.location.pathname);
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
