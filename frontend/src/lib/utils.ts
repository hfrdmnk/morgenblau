import { clsx } from 'clsx';
import type { ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

export function cn(...inputs: ClassValue[]) {
    return twMerge(clsx(inputs));
}

export function isMacPlatform(): boolean {
    if (typeof navigator === 'undefined') {
        return false;
    }

    const platform = navigator.platform || navigator.userAgent || '';

    return /Mac|iPhone|iPad/.test(platform);
}

// goBackOr steps back in history when we arrived from within the app, so closing
// a reader restores the previous page's scroll position (bfcache) and plays the
// reverse view transition. On a deep link (no same-origin referrer, or an empty
// stack) it navigates to the computed fallback instead.
export function goBackOr(fallbackHref: string): void {
    if (
        window.history.length > 1 &&
        document.referrer.startsWith(window.location.origin)
    ) {
        window.history.back();
        return;
    }
    window.location.href = fallbackHref;
}

const SAFE_SCHEMES = new Set(['http:', 'https:', 'mailto:']);

// safeHref drops URLs whose scheme isn't on the allowlist so untrusted strings
// (e.g. a PDS-supplied avatar or feed link) can't render as javascript: or data:.
export function safeHref(
    value: string | null | undefined,
): string | undefined {
    if (!value) {
        return undefined;
    }

    try {
        const url = new URL(value);

        return SAFE_SCHEMES.has(url.protocol) ? value : undefined;
    } catch {
        return undefined;
    }
}
