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
