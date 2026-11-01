import { useEffect, useState } from 'react';
import { toast } from 'sonner';

import { api, describeMutationError } from '@/lib/api';

type Setting = { enabled: boolean };

export function usePublicFollowing() {
    const [enabled, setEnabled] = useState<boolean>();
    const [busy, setBusy] = useState(false);
    useEffect(() => {
        const controller = new AbortController();
        void api<Setting>('/api/public-following', { signal: controller.signal })
            .then((setting) => setEnabled(setting.enabled))
            .catch(() => {
                if (!controller.signal.aborted)
                    toast.error('Could not load your public list setting.');
            });
        return () => controller.abort();
    }, []);

    async function toggle() {
        setBusy(true);
        try {
            const setting = await api<Setting>('/api/public-following', {
                method: 'PUT',
                body: { enabled: !enabled },
            });
            setEnabled(setting.enabled);
        } catch (error) {
            toast.error(
                describeMutationError(
                    error,
                    'Could not change your public list. Please try again.',
                ),
            );
        } finally {
            setBusy(false);
        }
    }

    return { enabled, busy, toggle };
}
