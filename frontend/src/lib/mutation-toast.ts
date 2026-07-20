import { toast } from 'sonner';

import { classifyMutationError, describeMutationError } from '@/lib/api';
import { PATHS } from '@/lib/paths';

// One reauth toast (copy + action) reused by every mutation call site so it can't drift out of sync.
export function toastMutationError(error: unknown, fallback: string): void {
    if (classifyMutationError(error) === 'reauth') {
        toast.error('Your session is out of date', {
            description: 'Sign in again to keep going.',
            action: {
                label: 'Sign in again',
                // Native navigation: reauth exits the authed shell, which app.tsx assumes is a full server round trip.
                onClick: () => window.location.assign(PATHS.login),
            },
        });
        return;
    }
    toast.error(describeMutationError(error, fallback));
}
