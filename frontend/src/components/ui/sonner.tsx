import { Toaster as Sonner } from 'sonner';

import { AlertIcon, CheckIcon, LoadingIcon } from '@/components/icons';

export function Toaster() {
    return (
        <Sonner
            position="top-center"
            icons={{
                success: <CheckIcon className="size-4 text-success-icon" />,
                error: <AlertIcon className="size-4 text-error-icon" />,
                warning: <AlertIcon className="size-4 text-warning" />,
                info: <AlertIcon className="size-4 text-neutral" />,
                loading: <LoadingIcon className="size-4 text-neutral" />,
            }}
            toastOptions={{
                unstyled: true,
                classNames: {
                    toast: 'flex w-full items-start gap-3 rounded-lg bg-popover p-4 font-sans text-sm text-popover-foreground shadow-popover',
                    icon: 'flex h-lh shrink-0 items-center',
                    content: 'flex min-w-0 flex-1 flex-col gap-1',
                    title: 'font-medium',
                    description: 'text-muted-foreground',
                },
            }}
        />
    );
}
