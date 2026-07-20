import { api } from '@/lib/api';

// Shared by the save and share toggles: clear local state before the DELETE lands, restore it if the request fails.
export function optimisticDelete(options: {
    path: string;
    clear: () => void;
    restore: () => void;
    onError?: (error: unknown) => void;
    settle: () => void;
}): void {
    options.clear();
    api(options.path, { method: 'DELETE' })
        .catch((error: unknown) => {
            options.restore();
            options.onError?.(error);
        })
        .finally(options.settle);
}
