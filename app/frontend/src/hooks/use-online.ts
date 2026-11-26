import { useSyncExternalStore } from 'react';

import { connection } from '@/lib/api';

// The browser can report online while Morgenblau is out of reach, so a failed request counts as offline too.
export function useOnline() {
    return useSyncExternalStore(
        (onChange) => {
            window.addEventListener('online', onChange);
            window.addEventListener('offline', onChange);
            connection.addEventListener('change', onChange);
            return () => {
                window.removeEventListener('online', onChange);
                window.removeEventListener('offline', onChange);
                connection.removeEventListener('change', onChange);
            };
        },
        () => navigator.onLine && connection.reachable,
    );
}
