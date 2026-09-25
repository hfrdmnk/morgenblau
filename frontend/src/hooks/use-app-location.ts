import { useBrowserLocation } from 'wouter/use-browser-location';

export function useAppLocation(options?: { ssrPath?: string }) {
    const [location, navigate] = useBrowserLocation(options);

    const navigateWithScrollReset: typeof navigate = (to, navOptions) => {
        navigate(to, navOptions);
        window.scrollTo(0, 0);
    };

    return [location, navigateWithScrollReset] as [
        typeof location,
        typeof navigateWithScrollReset,
    ];
}
