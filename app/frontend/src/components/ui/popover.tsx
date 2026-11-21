import { Popover as PopoverPrimitive } from '@base-ui/react/popover';
import type { ComponentProps } from 'react';

import { cn } from 'cn';

function Popover({ ...props }: PopoverPrimitive.Root.Props) {
    return <PopoverPrimitive.Root data-slot="popover" {...props} />;
}

// explain: a dotted underline marks text that opens an explanation, not an action.
const triggerVariants = {
    explain:
        'flex shrink-0 cursor-pointer items-center gap-1.5 rounded-sm underline decoration-subtle-line decoration-dotted underline-offset-4 transition-[color] duration-(--motion-duration-fast) hover:text-foreground',
};

function PopoverTrigger({ variant, ...props }: PopoverPrimitive.Trigger.Props & { variant?: keyof typeof triggerVariants }) {
    return <PopoverPrimitive.Trigger data-slot="popover-trigger" className={variant && triggerVariants[variant]} {...props} />;
}

function PopoverContent({
    className,
    align = 'center',
    alignOffset = 0,
    side = 'bottom',
    sideOffset = 8,
    ...props
}: PopoverPrimitive.Popup.Props &
    Pick<
        PopoverPrimitive.Positioner.Props,
        'align' | 'alignOffset' | 'side' | 'sideOffset'
    >) {
    return (
        <PopoverPrimitive.Portal>
            <PopoverPrimitive.Positioner
                align={align}
                alignOffset={alignOffset}
                side={side}
                sideOffset={sideOffset}
                className="isolate z-50"
            >
                <PopoverPrimitive.Popup
                    data-slot="popover-content"
                    className={cn(
                        'flex w-72 origin-(--transform-origin) flex-col gap-4 rounded-lg bg-popover p-4 text-sm text-popover-foreground shadow-popover transition-[opacity,transform] duration-(--motion-duration-overlay) ease-out data-[starting-style]:scale-98 data-[starting-style]:opacity-0 data-[ending-style]:scale-98 data-[ending-style]:opacity-0',
                        className,
                    )}
                    {...props}
                />
            </PopoverPrimitive.Positioner>
        </PopoverPrimitive.Portal>
    );
}

function PopoverHeader({ className, ...props }: ComponentProps<'div'>) {
    return (
        <div
            data-slot="popover-header"
            className={cn('flex flex-col gap-1', className)}
            {...props}
        />
    );
}

function PopoverTitle({ className, ...props }: PopoverPrimitive.Title.Props) {
    return (
        <PopoverPrimitive.Title
            data-slot="popover-title"
            className={cn('font-medium', className)}
            {...props}
        />
    );
}

function PopoverDescription({
    className,
    pretty = false,
    ...props
}: PopoverPrimitive.Description.Props & { pretty?: boolean }) {
    return (
        <PopoverPrimitive.Description
            data-slot="popover-description"
            className={cn('text-muted-foreground', pretty && 'text-pretty', className)}
            {...props}
        />
    );
}

export {
    Popover,
    PopoverContent,
    PopoverDescription,
    PopoverHeader,
    PopoverTitle,
    PopoverTrigger,
};
