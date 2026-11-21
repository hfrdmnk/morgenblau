import { cva, type VariantProps } from 'class-variance-authority';
import type { ComponentProps } from 'react';

import { cn } from 'cn';

// A whole list row as one button; its hover fill bleeds past the list's edges so the row's text lines up with the text around it.
const rowButtonVariants = cva(
    'flex cursor-pointer items-center text-left transition-[background-color] duration-(--motion-duration-fast) hover:bg-secondary-soft',
    {
        variants: {
            size: {
                default: '-mx-3 min-h-16 w-bleed-3 gap-3 rounded-lg p-3',
                compact: '-mx-2 h-9 w-bleed-2 justify-between gap-4 rounded-md px-2 text-sm disabled:pointer-events-none pointer-coarse:h-11',
            },
        },
        defaultVariants: { size: 'default' },
    },
);

function RowButton({ size, type = 'button', ...props }: Omit<ComponentProps<'button'>, 'className' | 'style'> & VariantProps<typeof rowButtonVariants>) {
    return <button data-slot="row-button" className={rowButtonVariants({ size })} type={type} {...props} />;
}

// Holds room for three compact rows while they load, so the content below does not move when they arrive.
function RowSpace({ reserve, ...props }: Omit<ComponentProps<'div'>, 'className' | 'style'> & { reserve: boolean }) {
    return <div data-slot="row-space" className={cn(reserve && 'min-h-27 pointer-coarse:min-h-33')} {...props} />;
}

export { RowButton, RowSpace };
