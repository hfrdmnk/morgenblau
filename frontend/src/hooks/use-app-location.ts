import { useEffect } from 'react';
import { useBrowserLocation, useHistoryState } from 'wouter/use-browser-location';

import { isAccountMenuPath } from '@/lib/paths';

export type AppNavigationState = {
    settingsReturn?: { href: string; scrollY: number };
    scrollY?: number;
};

export function useAppLocation(options?: { ssrPath?: string }) {
    const [location, navigate] = useBrowserLocation(options);

    const navigateWithScrollReset: typeof navigate = (to, navOptions) => {
        const previous: AppNavigationState | null = window.history.state;
        let state: AppNavigationState = navOptions?.state ?? {};
        if (isAccountMenuPath(new URL(to, window.location.href).pathname)) {
            state = {
                ...state,
                settingsReturn: isAccountMenuPath(location)
                    ? previous?.settingsReturn
                    : { href: window.location.pathname + window.location.search, scrollY: window.scrollY },
            };
        }
        navigate(to, { ...navOptions, state });
        window.scrollTo(0, state.scrollY ?? 0);
    };

    return [location, navigateWithScrollReset] as [
        typeof location,
        typeof navigateWithScrollReset,
    ];
}

export function useNavigationScrollRestoration() {
    const state = useHistoryState<AppNavigationState | null>();

    useEffect(() => {
        const target = state?.scrollY;
        if (!target) return;

        // The digest can be shorter than the saved position until its entries load.
        const observer = new ResizeObserver(() => {
            window.scrollTo(0, target);
            if (Math.abs(window.scrollY - target) < 1) observer.disconnect();
        });
        observer.observe(document.body);
        const cancel = () => observer.disconnect();
        window.addEventListener('wheel', cancel, { passive: true });
        window.addEventListener('touchstart', cancel, { passive: true });
        window.addEventListener('keydown', cancel);
        return () => {
            observer.disconnect();
            window.removeEventListener('wheel', cancel);
            window.removeEventListener('touchstart', cancel);
            window.removeEventListener('keydown', cancel);
        };
    }, [state]);
}
