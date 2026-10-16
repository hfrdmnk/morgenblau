import { toast } from 'sonner';

const KEEPS_ITS_KEYS = 'input, textarea, select, [contenteditable]:not([contenteditable="false"]), [role="dialog"]';

// Shift alone is the app's modifier: Ctrl, Alt and Meta combinations belong to the browser and the OS.
export function shortcutKey(event: KeyboardEvent): string | null {
    if (event.ctrlKey || event.altKey || event.metaKey) return null;
    if (event.repeat || event.isComposing || event.defaultPrevented) return null;
    if (event.target instanceof Element && event.target.closest(KEEPS_ITS_KEYS)) return null;
    if (!event.shiftKey) return event.key.length === 1 ? event.key.toLowerCase() : event.key;
    return `Shift+${event.key.length === 1 ? event.key.toUpperCase() : event.key}`;
}

export function showShortcutNotice(message: string) {
    toast.info(message, { id: 'shortcut-notice' });
}
