import { Toaster as Sonner } from 'sonner';

import { AlertIcon, ArrowRightIcon, CheckIcon, InfoIcon, LoadingIcon } from '@/components/icons';
import { useMediaQuery } from '@/hooks/use-media-query';

// Bottom-left keeps toasts off the right-hand sheet on wide screens; phones have a bottom sheet and nav, so toasts stay on top.
export function Toaster() {
    const wide = useMediaQuery('(min-width: 40rem)');
    return (
        <Sonner
            position={wide ? 'bottom-left' : 'top-center'}
            offset={{ top: 'calc(1.5rem + env(safe-area-inset-top))' }}
            mobileOffset={{ top: 'calc(1rem + env(safe-area-inset-top))' }}
            icons={{
                success: <CheckIcon className="size-4 text-success-icon" />,
                error: <AlertIcon className="size-4 text-error-icon" />,
                warning: <AlertIcon className="size-4 text-warning" />,
                info: <InfoIcon className="size-4 text-neutral" />,
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

export function LinkToast({ title, description, onClick }: { title: string; description: string; onClick: () => void }) {
    return (
        <button
            // Sonner draws the card around custom content; the button fills it, padding included, so the whole card is the target.
            className="group -m-4 flex w-[calc(100%+2rem)] cursor-pointer items-start gap-3 rounded-lg p-4 text-left font-normal"
            onClick={onClick}
            type="button"
        >
            <span className="flex h-lh shrink-0 items-center">
                <CheckIcon className="size-4 text-success-icon" />
            </span>
            <span className="flex min-w-0 flex-1 flex-col gap-1">
                <span className="flex items-center gap-1.5 font-medium">
                    {title}
                    <ArrowRightIcon className="size-3.5 text-muted-foreground transition-transform duration-(--motion-duration-fast) ease-out group-hover:translate-x-0.5" />
                </span>
                <span className="text-muted-foreground">{description}</span>
            </span>
        </button>
    );
}
