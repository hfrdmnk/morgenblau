import { Combobox as ComboboxPrimitive } from '@base-ui/react/combobox';

import { cn } from '@/lib/utils';

const Combobox = ComboboxPrimitive.Root;

const ComboboxInput = ComboboxPrimitive.Input;

function ComboboxContent({
    align = 'start',
    alignOffset = 0,
    side = 'bottom',
    sideOffset = 6,
    anchor,
    className,
    ...props
}: ComboboxPrimitive.Popup.Props &
    Pick<
        ComboboxPrimitive.Positioner.Props,
        'align' | 'alignOffset' | 'side' | 'sideOffset' | 'anchor'
    >) {
    return (
        <ComboboxPrimitive.Portal>
            <ComboboxPrimitive.Positioner
                className="isolate z-50 outline-none"
                align={align}
                alignOffset={alignOffset}
                anchor={anchor}
                side={side}
                sideOffset={sideOffset}
            >
                <ComboboxPrimitive.Popup
                    data-slot="combobox-content"
                    className={cn(
                        'max-h-(--available-height) w-(--anchor-width) max-w-(--available-width) origin-(--transform-origin) overflow-hidden rounded-lg bg-popover text-sm text-popover-foreground shadow-popover outline-none transition-[opacity,transform] duration-(--motion-duration-overlay) ease-out data-[starting-style]:scale-[0.98] data-[starting-style]:opacity-0 data-[ending-style]:scale-[0.98] data-[ending-style]:opacity-0',
                        className,
                    )}
                    {...props}
                />
            </ComboboxPrimitive.Positioner>
        </ComboboxPrimitive.Portal>
    );
}

function ComboboxList({ className, ...props }: ComboboxPrimitive.List.Props) {
    return (
        <ComboboxPrimitive.List
            data-slot="combobox-list"
            className={cn(
                'max-h-72 scroll-py-1 overflow-y-auto overscroll-contain p-1 data-empty:p-0',
                className,
            )}
            {...props}
        />
    );
}

function ComboboxItem({ className, ...props }: ComboboxPrimitive.Item.Props) {
    return (
        <ComboboxPrimitive.Item
            data-slot="combobox-item"
            className={cn(
                'flex min-h-9 cursor-default items-center gap-3 rounded-md px-2 outline-hidden select-none data-highlighted:bg-secondary data-disabled:pointer-events-none data-disabled:opacity-50',
                className,
            )}
            {...props}
        />
    );
}

export { Combobox, ComboboxContent, ComboboxInput, ComboboxItem, ComboboxList };
