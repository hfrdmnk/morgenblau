import { useState, useSyncExternalStore } from 'react';

export const readerFonts = ['sans', 'serif', 'mono'] as const;
export type ReaderFont = (typeof readerFonts)[number];
export const readerFontLabels: Record<ReaderFont, string> = { sans: 'Sans', serif: 'Serif', mono: 'Mono' };
const storageKey = 'morgenblau-reader-font';

export function parseReaderFont(value: string | null): ReaderFont {
    return readerFonts.find((font) => font === value) ?? 'sans';
}

// Every reader of the font follows a change made anywhere, such as from the command menu over an open entry.
const listeners = new Set<() => void>();

function storedFont(): ReaderFont | null {
    try {
        return parseReaderFont(localStorage.getItem(storageKey));
    } catch {
        return null;
    }
}

function subscribe(listener: () => void) {
    listeners.add(listener);
    return () => { listeners.delete(listener); };
}

export function useReaderFont() {
    const font = useSyncExternalStore(subscribe, () => storedFont() ?? 'sans');
    const [error, setError] = useState(() => (storedFont() === null ? 'Your browser could not load the saved reader font.' : ''));

    function changeFont(next: ReaderFont) {
        try {
            localStorage.setItem(storageKey, next);
            setError('');
            listeners.forEach((listener) => listener());
        } catch {
            setError('Your browser could not save the reader font. Your selection has not changed.');
        }
    }

    return [font, changeFont, error] as const;
}
