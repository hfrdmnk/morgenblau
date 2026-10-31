import { Button as ButtonPrimitive } from '@base-ui/react/button';
import { cva, type VariantProps } from 'class-variance-authority';

import { cn } from '@/lib/utils';

const raised =
    'relative bg-(--btn-surface) bg-linear-to-b from-(--btn-from) to-(--btn-surface) shadow-[inset_0_2px_0_0_var(--btn-highlight)] [--btn-from:color-mix(in_oklch,var(--btn-surface),white_15%)] [--btn-highlight:color-mix(in_oklch,var(--btn-surface),white_30%)] after:pointer-events-none after:absolute after:inset-0 after:rounded-[inherit] after:bg-linear-to-b after:from-[color-mix(in_oklch,var(--btn-from),black_10%)] after:to-[color-mix(in_oklch,var(--btn-surface),black_10%)] after:p-px after:[mask:linear-gradient(#000_0_0)_content-box_exclude,linear-gradient(#000_0_0)] hover:from-(--btn-highlight) aria-expanded:from-(--btn-highlight)';

const buttonVariants = cva(
    "inline-flex shrink-0 cursor-pointer items-center justify-center gap-1.5 rounded-md text-sm font-medium whitespace-nowrap transition-[background-color,color,transform,--tw-gradient-from] duration-(--motion-duration-fast) ease-out select-none motion-safe:active:not-focus-visible:scale-[0.98] disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-45 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
    {
        variants: {
            variant: {
                default: `${raised} [--btn-surface:var(--primary)] text-primary-foreground`,
                secondary: `${raised} [--btn-surface:var(--secondary)] text-secondary-foreground`,
                ghost: 'hover:bg-secondary aria-expanded:bg-secondary',
                destructive: `${raised} [--btn-surface:color-mix(in_oklab,var(--destructive)_12%,var(--background))] text-destructive dark:[--btn-surface:color-mix(in_oklab,var(--destructive)_18%,var(--background))]`,
            },
            size: {
                default: 'h-10 px-3.5',
                sm: 'h-8 rounded-sm px-2.5 text-xs',
                lg: 'h-11 px-4',
                icon: 'size-10',
                'icon-sm': 'relative size-8 rounded-sm before:absolute before:-inset-1.5',
                'icon-lg': 'size-11',
            },
        },
        defaultVariants: {
            variant: 'default',
            size: 'default',
        },
    },
);

function Button({
    className,
    variant = 'default',
    size = 'default',
    ...props
}: ButtonPrimitive.Props & VariantProps<typeof buttonVariants>) {
    return (
        <ButtonPrimitive
            data-slot="button"
            className={cn(buttonVariants({ variant, size }), className)}
            {...props}
        />
    );
}

export { Button };
