import { api } from '@/lib/api';

// Mirrors the limits in internal/api/feedback.go, so a reader hears about them before sending.
export const MAX_FEEDBACK_IMAGES = 5;
export const MAX_FEEDBACK_IMAGE_BYTES = 12 * 1024 * 1024;
export const FEEDBACK_IMAGE_TYPES = ['image/png', 'image/jpeg', 'image/gif', 'image/webp'];

// What only the browser knows; the server adds the account and the user agent.
function browserContext(): [string, string][] {
    const media = (query: string) => window.matchMedia(query).matches;
    return [
        ['Page', location.href],
        ['Title', document.title],
        ['Referrer', document.referrer || 'none'],
        ['Viewport', `${innerWidth} × ${innerHeight} @${devicePixelRatio}x`],
        ['Screen', `${screen.width} × ${screen.height}`],
        ['Theme', document.documentElement.classList.contains('dark') ? 'dark' : 'light'],
        ['Reduced motion', media('(prefers-reduced-motion: reduce)') ? 'yes' : 'no'],
        ['Pointer', media('(pointer: coarse)') ? 'touch' : 'fine'],
        ['Display mode', media('(display-mode: standalone)') ? 'installed app' : 'browser'],
        ['Service worker', navigator.serviceWorker?.controller ? 'active' : 'none'],
        ['Online', navigator.onLine ? 'yes' : 'no'],
        ['Languages', navigator.languages.join(', ')],
        ['Time zone', Intl.DateTimeFormat().resolvedOptions().timeZone],
        ['Local time', new Date().toString()],
    ];
}

export function sendFeedback(message: string, images: File[], signal?: AbortSignal) {
    const form = new FormData();
    form.append('message', message);
    form.append('context', JSON.stringify(browserContext()));
    for (const image of images) form.append('images', image, image.name);
    return api('/api/feedback', { method: 'POST', body: form, signal });
}
