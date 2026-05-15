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
