import { useCallback, useEffect, useState } from 'react';

import { PeoplePanel } from '@/components/discover/people-panel';
import { SourcesPanel } from '@/components/discover/sources-panel';
import { Subnav } from '@/components/subnav';
import { useDocumentTitle } from '@/hooks/use-document-title';

type Subtab = 'sources' | 'people';

const SUBTABS: { id: Subtab; label: string }[] = [
    { id: 'sources', label: 'Sources' },
    { id: 'people', label: 'People' },
];

function readTabFromURL(): Subtab {
    return new URLSearchParams(window.location.search).get('tab') === 'people'
        ? 'people'
        : 'sources';
}

export function Discover() {
    useDocumentTitle('Discover');
    const [active, setActive] = useState<Subtab>(() => readTabFromURL());

    // Clean up the URL once on mount if the tab param was missing-equivalent (anything but 'people').
    useEffect(() => {
        const raw = new URLSearchParams(window.location.search).get('tab');
        if (raw === null || raw === 'people') return;
        const url = new URL(window.location.href);
        url.searchParams.delete('tab');
        window.history.replaceState(null, '', url.toString());
    }, []);

    useEffect(() => {
        const onPopState = () => {
            setActive(readTabFromURL());
        };
        window.addEventListener('popstate', onPopState);
        return () => window.removeEventListener('popstate', onPopState);
    }, []);

    const handleSelect = useCallback((id: string) => {
        const tab: Subtab = id === 'people' ? 'people' : 'sources';
        setActive(tab);
        const url = new URL(window.location.href);
        if (tab === 'people') {
            url.searchParams.set('tab', 'people');
        } else {
            url.searchParams.delete('tab');
        }
        window.history.pushState(null, '', url.toString());
    }, []);

    return (
        <div className="mx-auto w-full max-w-2xl px-4 pt-10 pb-12 sm:px-6">
            <header className="mb-6">
                <Subnav
                    items={SUBTABS}
                    activeId={active}
                    onSelect={handleSelect}
                    ariaLabel="Discover section"
                />
            </header>
            {active === 'people' ? <PeoplePanel /> : <SourcesPanel />}
        </div>
    );
}
