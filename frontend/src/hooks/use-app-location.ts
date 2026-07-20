import { useBrowserLocation } from 'wouter/use-browser-location';

let spaNavigated = false;
export function hasNavigatedInApp(): boolean {
    return spaNavigated;
}

// The single location hook passed to <Router>; adds the scroll reset a full document load used to provide.
export function useAppLocation(options?: { ssrPath?: string }) {
    const [location, navigate] = useBrowserLocation(options);

    const navigateWithScrollReset: typeof navigate = (to, navOptions) => {
        spaNavigated = true;
        navigate(to, navOptions);
        window.scrollTo(0, 0);
    };

    // Not `as const`: BaseLocationHook requires a mutable tuple, and `as const` makes it readonly.
    return [location, navigateWithScrollReset] as [
        typeof location,
        typeof navigateWithScrollReset,
    ];
}
