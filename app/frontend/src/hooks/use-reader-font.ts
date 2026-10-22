import { useState } from 'react';

type ReaderFont = 'sans' | 'serif';
const storageKey = 'morgenblau-reader-font';

export function useReaderFont() {
    const [state, setState] = useState<{ font: ReaderFont; error: string }>(() => {
        try {
            return { font: localStorage.getItem(storageKey) === 'serif' ? 'serif' : 'sans', error: '' };
        } catch {
            return { font: 'sans', error: 'Your browser could not load the saved reader font.' };
        }
    });

    function changeFont(next: ReaderFont) {
        try {
            localStorage.setItem(storageKey, next);
            setState({ font: next, error: '' });
        } catch {
            setState((previous) => ({ ...previous, error: 'Your browser could not save the reader font. Your selection has not changed.' }));
        }
    }

    return [state.font, changeFont, state.error] as const;
}
