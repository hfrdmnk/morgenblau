import { toast } from 'sonner';

const KEEPS_ITS_KEYS = 'input, textarea, select, [contenteditable]:not([contenteditable="false"]), [role="dialog"]';

// Shift alone is the app's modifier: Ctrl, Alt and Meta combinations belong to the browser and the OS, except ⌘K (isCommandMenuKey).
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

// The command menu takes ⌘K, or Ctrl+K off Apple platforms, from anywhere, text fields included, as other apps do.
export function isCommandMenuKey(event: KeyboardEvent): boolean {
    return (event.metaKey || event.ctrlKey) && !event.altKey && !event.shiftKey && !event.repeat && event.key.toLowerCase() === 'k';
}

// A space separates keys that do the same thing.
export const SHORTCUTS = [
    { keys: 'Mod+K', label: 'Command menu', where: 'Anywhere' },
    { keys: 'Shift+D', label: 'Digest', where: 'Anywhere' },
    { keys: 'Shift+S', label: 'Sources', where: 'Anywhere' },
    { keys: 'Shift+L', label: 'Library', where: 'Anywhere' },
    { keys: 'ArrowLeft h', label: 'Previous day', where: 'Digest' },
    { keys: 'ArrowRight l', label: 'Next day', where: 'Digest' },
    { keys: 'Shift+T', label: 'Today', where: 'Digest' },
    { keys: 'ArrowUp k', label: 'Previous post', where: 'Post lists' },
    { keys: 'ArrowDown j', label: 'Next post', where: 'Post lists' },
    { keys: 'Enter', label: 'Open post', where: 'Post lists' },
    { keys: 'ArrowLeft k', label: 'Previous entry', where: 'Reader' },
    { keys: 'ArrowRight j', label: 'Next entry', where: 'Reader' },
    { keys: 'Escape', label: 'Back', where: 'Reader' },
    { keys: 't', label: 'Add tags after saving', where: 'Reader' },
    { keys: 'ArrowUp', label: 'Previous passage', where: 'Video transcript' },
    { keys: 'ArrowDown', label: 'Next passage', where: 'Video transcript' },
    { keys: 'Enter', label: 'Play from passage', where: 'Video transcript' },
];
