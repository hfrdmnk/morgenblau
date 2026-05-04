import { router } from '@inertiajs/react';
import { useEffect } from 'react';
import { toast } from 'sonner';

export function useFlashToast(): void {
    useEffect(() => {
        return router.on('flash', (event) => {
            const flash = (event as CustomEvent).detail?.flash as
                | App.Data.Shared.FlashData
                | undefined;
            const data = flash?.toast;

            if (!data) {
                return;
            }

            toast[data.type](data.message, {
                id: `flash:${data.type}:${data.message}`,
            });
        });
    }, []);
}
