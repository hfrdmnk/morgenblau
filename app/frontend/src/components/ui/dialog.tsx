import { Dialog } from '@base-ui/react/dialog';

import { cn } from 'cn';

// Base UI renders no backdrop for a nested dialog unless forceRender is set:
// https://base-ui.com/react/components/dialog#nested-dialogs
function DialogBackdrop({ className, ...props }: Dialog.Backdrop.Props) {
    return (
        <Dialog.Backdrop
            data-slot="dialog-backdrop"
            forceRender
            className={cn(
                'fixed inset-0 z-50 bg-scrim backdrop-blur-xs backdrop-saturate-180 transition-[opacity,backdrop-filter] duration-(--motion-duration-overlay) ease-out data-[starting-style]:opacity-0 data-[starting-style]:backdrop-blur-none data-[starting-style]:backdrop-saturate-100 data-[ending-style]:opacity-0 data-[ending-style]:backdrop-blur-none data-[ending-style]:backdrop-saturate-100',
                className,
            )}
            {...props}
        />
    );
}

function DialogPopup({
    className,
    size,
    short = false,
    ...props
}: Dialog.Popup.Props & { size?: 'md'; short?: boolean }) {
    return (
        <Dialog.Popup
            data-slot="dialog-popup"
            className={cn(
                'fixed top-1/2 left-1/2 z-50 max-h-(--dialog-max-height) w-(--dialog-width) -translate-x-1/2 -translate-y-1/2 origin-center overflow-y-auto overscroll-contain rounded-xl bg-popover p-6 text-popover-foreground shadow-popover transition-[opacity,scale] duration-(--motion-duration-overlay) ease-out data-[starting-style]:scale-96 data-[starting-style]:opacity-0 data-[ending-style]:scale-96 data-[ending-style]:opacity-0',
                size === 'md' && 'max-w-md',
                short && 'max-h-(--dialog-short-max-height)',
                className,
            )}
            {...props}
        />
    );
}

export { DialogBackdrop, DialogPopup };
