import { api } from '@/lib/api';

// Clear local state before the DELETE lands, then restore it if the request fails.
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
