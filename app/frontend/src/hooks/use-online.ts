import { useSyncExternalStore } from 'react';

import { connection } from '@/lib/api';

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
