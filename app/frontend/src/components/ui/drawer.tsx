import { Drawer } from '@base-ui/react/drawer';
import type { ComponentProps } from 'react';

import { cn } from 'cn';

type Side = 'right' | 'bottom';

function DrawerBackdrop({ className, ...props }: Drawer.Backdrop.Props) {
    return (
        <Drawer.Backdrop
            data-slot="drawer-backdrop"
            className={cn(
                'fixed inset-0 z-40 bg-scrim drawer-fade backdrop-blur-xs backdrop-saturate-180 transition-[opacity,backdrop-filter] duration-(--motion-duration-overlay) ease-out data-[starting-style]:opacity-0 data-[ending-style]:opacity-0 data-swiping:duration-0',
                className,
            )}
            {...props}
        />
    );
}

function DrawerViewport({ side, className, ...props }: Drawer.Viewport.Props & { side: Side }) {
    return (
        <Drawer.Viewport
            data-slot="drawer-viewport"
            className={cn('fixed inset-0 z-50 flex', side === 'right' ? 'justify-end p-2' : 'items-end', className)}
            {...props}
        />
    );
}

// Slides from the edge it is swiped back to; a swipe drives the transform directly, so its transition pauses while swiping.
function DrawerPopup({ side, className, ...props }: Drawer.Popup.Props & { side: Side }) {
    return (
        <Drawer.Popup
            data-slot="drawer-popup"
            className={cn(
                'flex flex-col bg-popover text-popover-foreground shadow-popover outline-none transition-transform duration-(--motion-duration-overlay) ease-(--ease-out) data-swiping:duration-0 data-swiping:select-none data-[ending-style]:drawer-release',
                side === 'right'
                    ? 'h-full w-md max-w-full rounded-xl drawer-slide-x'
                    : 'max-h-(--sheet-max-height) w-full rounded-t-2xl pb-(--safe-bottom) drawer-slide-y',
                className,
            )}
            {...props}
        />
    );
}

// The grab handle a bottom sheet shows above its content.
function DrawerHandle() {
    return (
        <div
            aria-hidden="true"
            data-slot="drawer-handle"
            className="mx-auto mt-2 h-1 w-10 shrink-0 rounded-full bg-accent"
        />
    );
}

// The scrolling body under a sheet's header; min-h-0 lets it shrink inside the popup's column so it, not the sheet, scrolls.
function DrawerContent({ className, ...props }: Drawer.Content.Props) {
    return (
        <Drawer.Content
            data-slot="drawer-content"
            className={cn('min-h-0 flex-1 overflow-y-auto overscroll-contain px-6 pb-6', className)}
            {...props}
        />
    );
}

// Sticks to the bottom of DrawerContent, bleeding across its padding, so a screen taller than the sheet keeps its actions in view.
function DrawerFooter({ className, ...props }: ComponentProps<'div'>) {
    return (
        <div
            data-slot="drawer-footer"
            className={cn('sticky bottom-0 -mx-6 flex flex-col gap-2 bg-popover px-6 pt-2 pb-1', className)}
            {...props}
        />
    );
}

export { DrawerBackdrop, DrawerContent, DrawerFooter, DrawerHandle, DrawerPopup, DrawerViewport };
