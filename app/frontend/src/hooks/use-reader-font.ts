import { useState, useSyncExternalStore } from 'react';

type ReaderFont = 'sans' | 'serif';
const storageKey = 'morgenblau-reader-font';
// Every reader of the font follows a change made anywhere, such as from the command menu over an open entry.
const listeners = new Set<() => void>();

function storedFont(): ReaderFont | null {
    try {
        return localStorage.getItem(storageKey) === 'serif' ? 'serif' : 'sans';
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
