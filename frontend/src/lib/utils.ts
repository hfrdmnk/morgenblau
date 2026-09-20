import { clsx } from 'clsx';
import type { ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

export function cn(...inputs: ClassValue[]) {
    return twMerge(clsx(inputs));
}

const SAFE_SCHEMES = new Set(['http:', 'https:', 'mailto:']);

// Drops URLs off the allowlist so untrusted strings (e.g. a PDS-supplied link) can't render as javascript: or data:.
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

export function hostnameOf(url: string): string | null {
    try {
        const u = new URL(url);
        return u.hostname.replace(/^www\./, '');
    } catch {
        return null;
    }
}

// True only for an unmodified left click, so middle-click/new-tab keeps native link behavior.
export function isPlainLeftClick(e: {
    button: number;
    metaKey: boolean;
    ctrlKey: boolean;
    shiftKey: boolean;
}): boolean {
    return e.button === 0 && !e.metaKey && !e.ctrlKey && !e.shiftKey;
}
