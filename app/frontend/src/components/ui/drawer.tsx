import { Drawer } from '@base-ui/react/drawer';

import { cn } from '@/lib/utils';

type Side = 'right' | 'bottom';

function DrawerBackdrop({ className, ...props }: Drawer.Backdrop.Props) {
    return (
        <Drawer.Backdrop
            data-slot="drawer-backdrop"
            className={cn(
                'fixed inset-0 z-40 bg-background/40 opacity-[calc(1-var(--drawer-swipe-progress))] backdrop-blur-xs backdrop-saturate-180 transition-[opacity,backdrop-filter] duration-(--motion-duration-overlay) ease-out data-[starting-style]:opacity-0 data-[ending-style]:opacity-0 data-swiping:duration-0',
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
                'flex flex-col bg-popover text-popover-foreground shadow-popover outline-none transition-transform duration-(--motion-duration-overlay) ease-(--ease-out) data-swiping:duration-0 data-swiping:select-none data-[ending-style]:duration-[calc(var(--drawer-swipe-strength)*var(--motion-duration-overlay))]',
                side === 'right'
                    ? 'h-full w-[28rem] max-w-[calc(100vw-1rem)] rounded-xl [transform:translateX(var(--drawer-swipe-movement-x))] data-[ending-style]:[transform:translateX(calc(100%+1rem))] data-[starting-style]:[transform:translateX(calc(100%+1rem))]'
                    : 'max-h-[92dvh] w-full rounded-t-2xl pb-[env(safe-area-inset-bottom)] [transform:translateY(var(--drawer-swipe-movement-y))] data-[ending-style]:[transform:translateY(100%)] data-[starting-style]:[transform:translateY(100%)]',
                className,
            )}
            {...props}
        />
    );
}

export { DrawerBackdrop, DrawerPopup, DrawerViewport };
