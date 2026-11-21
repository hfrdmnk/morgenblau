import { Autocomplete as AutocompletePrimitive } from '@base-ui/react/autocomplete';
import { Combobox as ComboboxPrimitive } from '@base-ui/react/combobox';

import { cn } from 'cn';

const Combobox = ComboboxPrimitive.Root;

// Autocomplete keeps free-form input text as the value; its other parts are the combobox parts below.
const Autocomplete = AutocompletePrimitive.Root;

const ComboboxInput = ComboboxPrimitive.Input;

function ComboboxContent({
    align = 'start',
    alignOffset = 0,
    side = 'bottom',
    sideOffset = 6,
    anchor,
    hideEmpty = false,
    className,
    ...props
}: ComboboxPrimitive.Popup.Props &
    Pick<
        ComboboxPrimitive.Positioner.Props,
        'align' | 'alignOffset' | 'side' | 'sideOffset' | 'anchor'
    > & { hideEmpty?: boolean }) {
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
                        'max-h-(--available-height) w-(--anchor-width) max-w-(--available-width) origin-(--transform-origin) overflow-hidden rounded-lg bg-popover text-sm text-popover-foreground shadow-popover outline-none transition-[opacity,transform] duration-(--motion-duration-overlay) ease-out data-[starting-style]:scale-98 data-[starting-style]:opacity-0 data-[ending-style]:scale-98 data-[ending-style]:opacity-0',
                        hideEmpty && 'data-empty:hidden',
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

// padded: an item whose content stacks two lines, such as a name over a handle, which would otherwise touch the highlight's edges.
function ComboboxItem({ className, padded = false, ...props }: ComboboxPrimitive.Item.Props & { padded?: boolean }) {
    return (
        <ComboboxPrimitive.Item
            data-slot="combobox-item"
            className={cn(
                'flex min-h-9 cursor-default items-center gap-3 rounded-md px-2 outline-hidden select-none data-highlighted:bg-secondary data-disabled:pointer-events-none data-disabled:opacity-50',
                padded && 'py-1.5',
                className,
            )}
            {...props}
        />
    );
}

export { Autocomplete, Combobox, ComboboxContent, ComboboxInput, ComboboxItem, ComboboxList };
