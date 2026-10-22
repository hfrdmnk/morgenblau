import { Button as ButtonPrimitive } from '@base-ui/react/button';
import { cva, type VariantProps } from 'class-variance-authority';

import { cn } from '@/lib/utils';

const buttonVariants = cva(
    "inline-flex shrink-0 cursor-pointer items-center justify-center gap-1.5 rounded-md text-sm font-medium whitespace-nowrap transition-[background-color,color,transform] duration-(--motion-duration-fast) ease-out select-none motion-safe:active:not-focus-visible:scale-[0.98] disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-45 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
    {
        variants: {
            variant: {
                default: 'bg-primary text-primary-foreground hover:bg-primary/90',
                secondary:
                    'bg-secondary text-secondary-foreground hover:bg-accent',
                ghost: 'hover:bg-secondary aria-expanded:bg-secondary',
                destructive:
                    'bg-destructive/10 text-destructive hover:bg-destructive/20 dark:bg-destructive/15 dark:hover:bg-destructive/25',
            },
            size: {
                default: 'h-10 px-3.5',
                sm: 'h-8 rounded-sm px-2.5 text-xs',
                lg: 'h-11 px-4',
                icon: 'size-10',
                'icon-sm': 'size-8 rounded-sm',
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
