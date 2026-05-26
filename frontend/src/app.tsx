import type { FC, ReactNode } from 'react';

import { MeProvider } from '@/components/me-provider';
import { WindowLayout } from '@/layouts/window-layout';
import { PATHS } from '@/lib/paths';
import { Digest } from '@/pages/digest';
import { Discover } from '@/pages/discover';
import { Entry } from '@/pages/entry';
import { Library } from '@/pages/library';
import { Login } from '@/pages/login';
import { Source } from '@/pages/source';
import { Sources } from '@/pages/sources';
import { Welcome } from '@/pages/welcome';

type PageDef = { Component: FC; authed: boolean; chrome: boolean };

const pages: Record<string, PageDef> = {
    [PATHS.welcome]: { Component: Welcome, authed: false, chrome: false },
    [PATHS.login]: { Component: Login, authed: false, chrome: false },
    [PATHS.digest]: { Component: Digest, authed: true, chrome: true },
    [PATHS.library]: { Component: Library, authed: true, chrome: true },
    [PATHS.sources]: { Component: Sources, authed: true, chrome: true },
    [PATHS.discover]: { Component: Discover, authed: true, chrome: true },
};

const entryDef: PageDef = { Component: Entry, authed: true, chrome: false };
const sourceDef: PageDef = { Component: Source, authed: true, chrome: false };

function resolvePage(pathname: string): PageDef | null {
    if (pages[pathname]) return pages[pathname];
    if (pathname.startsWith(`${PATHS.entry}/`)) return entryDef;
    if (pathname.startsWith(`${PATHS.sources}/`)) return sourceDef;
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
