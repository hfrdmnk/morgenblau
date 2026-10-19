import { Menu as MenuPrimitive } from '@base-ui/react/menu';

import { cn } from '@/lib/utils';

function DropdownMenu({ ...props }: MenuPrimitive.Root.Props) {
    return <MenuPrimitive.Root data-slot="dropdown-menu" {...props} />;
}

function DropdownMenuTrigger({ ...props }: MenuPrimitive.Trigger.Props) {
    return (
        <MenuPrimitive.Trigger data-slot="dropdown-menu-trigger" {...props} />
    );
}

function DropdownMenuContent({
    align = 'end',
    alignOffset = 0,
    side = 'bottom',
    sideOffset = 4,
    className,
    ...props
}: MenuPrimitive.Popup.Props &
    Pick<
        MenuPrimitive.Positioner.Props,
        'align' | 'alignOffset' | 'side' | 'sideOffset'
    >) {
    return (
        <MenuPrimitive.Portal>
            <MenuPrimitive.Positioner
                className="isolate z-50 outline-none"
                align={align}
                alignOffset={alignOffset}
                side={side}
                sideOffset={sideOffset}
            >
                <MenuPrimitive.Popup
                    data-slot="dropdown-menu-content"
                    className={cn(
                        'max-h-(--available-height) min-w-44 origin-(--transform-origin) overflow-y-auto rounded-lg bg-popover p-1 text-sm text-popover-foreground shadow-popover outline-none transition-[opacity,transform] duration-(--motion-duration-overlay) ease-out data-[starting-style]:scale-[0.98] data-[starting-style]:opacity-0 data-[ending-style]:scale-[0.98] data-[ending-style]:opacity-0',
                        className,
                    )}
                    {...props}
                />
            </MenuPrimitive.Positioner>
        </MenuPrimitive.Portal>
    );
}

function DropdownMenuItem({
    className,
    variant = 'default',
    ...props
}: MenuPrimitive.Item.Props & {
    variant?: 'default' | 'destructive';
}) {
    return (
        <MenuPrimitive.Item
            data-slot="dropdown-menu-item"
            data-variant={variant}
            className={cn(
                "flex min-h-9 cursor-default items-center gap-3 rounded-md px-2 outline-hidden select-none data-highlighted:bg-secondary data-[variant=destructive]:text-destructive data-[variant=destructive]:data-highlighted:bg-destructive/10 data-disabled:pointer-events-none data-disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
                className,
            )}
            {...props}
        />
    );
}

function DropdownMenuSeparator({
    className,
    ...props
}: MenuPrimitive.Separator.Props) {
    return (
        <MenuPrimitive.Separator
            data-slot="dropdown-menu-separator"
            className={cn('-mx-1 my-1 h-px bg-border', className)}
            {...props}
        />
    );
}

export {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuSeparator,
    DropdownMenuTrigger,
};
