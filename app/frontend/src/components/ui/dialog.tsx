import { Dialog } from '@base-ui/react/dialog';

import { cn } from 'cn';

// Base UI skips a nested dialog's backdrop, so a confirm opened from a sheet would leave the sheet unblurred.
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

// Centred on both axes and scaled from the centre, so every modal enters the same way.
// short: a popup whose content can grow long, such as an import receipt, stops well short of the screen's height.
function DialogPopup({ className, size, short = false, ...props }: Dialog.Popup.Props & { size?: 'md'; short?: boolean }) {
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
