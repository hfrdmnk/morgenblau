import { useEffect, useEffectEvent } from 'react';
import { useLocation } from 'wouter';

import { APP_MODES } from '@/lib/paths';
import { shortcutKey } from '@/lib/shortcuts';

export function useShortcuts(shortcuts: Partial<Record<string, () => void>>) {
    const onKeyDown = useEffectEvent((event: KeyboardEvent) => {
        const run = shortcuts[shortcutKey(event) ?? ''];
        if (!run) return;
        event.preventDefault();
        run();
    });

    useEffect(() => {
        const listener = (event: KeyboardEvent) => onKeyDown(event);
        window.addEventListener('keydown', listener);
        return () => window.removeEventListener('keydown', listener);
    }, []);
}

export function useModeShortcuts() {
    const [location, navigate] = useLocation();
    useShortcuts(Object.fromEntries(APP_MODES.map(({ href, shortcut }) => [
        shortcut,
        () => { if (location !== href) navigate(href); },
    ])));
}
