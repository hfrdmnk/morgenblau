import { useCallback, useEffect, useState } from 'react';

function readTabFromURL<Tab extends string>(
    tabs: readonly Tab[],
    defaultTab: Tab,
): Tab {
    const raw = new URLSearchParams(window.location.search).get('tab');
    return raw !== null && tabs.includes(raw as Tab) ? (raw as Tab) : defaultTab;
}

// Drives a Subnav from the `tab` query param. The default tab always carries no
// param; every other valid tab is reflected in the URL and synced on popstate.
export function useSubnavTab<Tab extends string>(
    tabs: readonly Tab[],
    defaultTab: Tab,
): { active: Tab; handleSelect: (id: string) => void } {
    const [active, setActive] = useState<Tab>(() =>
        readTabFromURL(tabs, defaultTab),
    );

    // Clean up the URL once on mount if the tab param isn't a valid non-default tab.
    useEffect(() => {
        const raw = new URLSearchParams(window.location.search).get('tab');
        if (
            raw === null ||
            (tabs.includes(raw as Tab) && raw !== defaultTab)
        ) {
            return;
        }
        const url = new URL(window.location.href);
        url.searchParams.delete('tab');
        window.history.replaceState(null, '', url.toString());
    }, [tabs, defaultTab]);

    useEffect(() => {
        const onPopState = () => {
            setActive(readTabFromURL(tabs, defaultTab));
        };
        window.addEventListener('popstate', onPopState);
        return () => window.removeEventListener('popstate', onPopState);
    }, [tabs, defaultTab]);

    const handleSelect = useCallback(
        (id: string) => {
            const tab = tabs.includes(id as Tab) ? (id as Tab) : defaultTab;
            setActive(tab);
            const url = new URL(window.location.href);
            if (tab === defaultTab) {
                url.searchParams.delete('tab');
            } else {
                url.searchParams.set('tab', tab);
            }
            window.history.pushState(null, '', url.toString());
        },
        [tabs, defaultTab],
    );

    return { active, handleSelect };
}
