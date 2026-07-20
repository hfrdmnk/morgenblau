import { useLocation } from 'wouter';

import { hasNavigatedInApp } from '@/hooks/use-app-location';

// history.back() (not a fallback navigate) preserves scroll position; deep links (empty in-app history) fall back to a normal navigation instead.
export function useGoBackOr(): (fallbackHref: string) => void {
    const [, navigate] = useLocation();
    return (fallbackHref) => {
        if (window.history.length > 1 && hasNavigatedInApp()) {
            window.history.back();
            return;
        }
        navigate(fallbackHref);
    };
}
