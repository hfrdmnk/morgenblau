import { ContextMenu as ContextMenuPrimitive } from '@base-ui/react/context-menu';
import { cva, type VariantProps } from 'class-variance-authority';

function ContextMenu(props: ContextMenuPrimitive.Root.Props) {
    return <ContextMenuPrimitive.Root data-slot="context-menu" {...props} />;
}

function ContextMenuTrigger(props: Omit<ContextMenuPrimitive.Trigger.Props, 'className' | 'style'>) {
    return <ContextMenuPrimitive.Trigger data-slot="context-menu-trigger" {...props} />;
}

function ContextMenuContent(props: Omit<ContextMenuPrimitive.Popup.Props, 'className' | 'style'>) {
    return (
        <ContextMenuPrimitive.Portal>
            <ContextMenuPrimitive.Positioner className="isolate z-50 outline-none">
                <ContextMenuPrimitive.Popup
                    data-slot="context-menu-content"
                    className="flex min-w-52 origin-(--transform-origin) flex-col rounded-lg bg-popover p-1 text-sm text-popover-foreground shadow-popover outline-none transition-[opacity,transform] duration-(--motion-duration-overlay) ease-out select-none data-[starting-style]:scale-98 data-[starting-style]:opacity-0 data-[ending-style]:scale-98 data-[ending-style]:opacity-0"
                    {...props}
                />
            </ContextMenuPrimitive.Positioner>
        </ContextMenuPrimitive.Portal>
    );
}

const itemVariants = cva(
    "flex min-h-9 cursor-default items-center gap-3 rounded-md px-2 outline-hidden data-highlighted:bg-secondary [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
    {
        variants: {
            tone: {
                default: '[&_svg]:text-muted-foreground',
                destructive: 'text-destructive',
            },
        },
        defaultVariants: { tone: 'default' },
    },
);

function ContextMenuItem({
    tone,
    ...props
}: Omit<ContextMenuPrimitive.Item.Props, 'className' | 'style'> & VariantProps<typeof itemVariants>) {
    return <ContextMenuPrimitive.Item data-slot="context-menu-item" className={itemVariants({ tone })} {...props} />;
}

function ContextMenuSeparator() {
    return <ContextMenuPrimitive.Separator data-slot="context-menu-separator" className="-mx-1 my-1 h-px bg-border" />;
}

export { ContextMenu, ContextMenuContent, ContextMenuItem, ContextMenuSeparator, ContextMenuTrigger };
