import { Dialog } from '@base-ui/react/dialog';

import { cn } from '@/lib/utils';

function DialogBackdrop({ className, ...props }: Dialog.Backdrop.Props) {
    return (
        <Dialog.Backdrop
            data-slot="dialog-backdrop"
            className={cn(
                'fixed inset-0 z-40 bg-background/40 backdrop-blur-xs backdrop-saturate-180 transition-[opacity,backdrop-filter] duration-(--motion-duration-overlay) ease-out data-[starting-style]:opacity-0 data-[starting-style]:backdrop-blur-none data-[starting-style]:backdrop-saturate-100 data-[ending-style]:opacity-0 data-[ending-style]:backdrop-blur-none data-[ending-style]:backdrop-saturate-100',
                className,
            )}
            {...props}
        />
    );
}

// Centred on both axes and scaled from the centre, so every modal enters the same way.
function DialogPopup({ className, ...props }: Dialog.Popup.Props) {
    return (
        <Dialog.Popup
            data-slot="dialog-popup"
            className={cn(
                'fixed top-1/2 left-1/2 z-50 max-h-[calc(100dvh-2rem)] w-[calc(100%-2rem)] -translate-x-1/2 -translate-y-1/2 origin-center overflow-y-auto rounded-xl bg-popover p-6 text-popover-foreground shadow-popover transition-[opacity,scale] duration-(--motion-duration-overlay) ease-out data-[starting-style]:scale-96 data-[starting-style]:opacity-0 data-[ending-style]:scale-96 data-[ending-style]:opacity-0',
                className,
            )}
            {...props}
        />
    );
}

export { DialogBackdrop, DialogPopup };
