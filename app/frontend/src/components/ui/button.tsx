import { Button as ButtonPrimitive } from '@base-ui/react/button';
import { cva, type VariantProps } from 'class-variance-authority';

import { cn } from 'cn';

const raised =
    'raised relative bg-(--btn-surface) bg-linear-to-b from-(--btn-from) to-(--btn-surface) shadow-raised hover:from-(--btn-highlight) aria-expanded:from-(--btn-highlight)';

const buttonVariants = cva(
    "inline-flex shrink-0 cursor-pointer items-center justify-center gap-1.5 rounded-md text-sm font-medium whitespace-nowrap transition-[background-color,color,transform,--tw-gradient-from] duration-(--motion-duration-fast) ease-out select-none motion-safe:active:not-focus-visible:scale-98 disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-45 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
    {
        variants: {
            variant: {
                default: `${raised} btn-surface-primary text-primary-foreground`,
                secondary: `${raised} btn-surface-secondary text-secondary-foreground`,
                ghost: 'hover:bg-secondary aria-expanded:bg-secondary',
                quiet: 'text-muted-foreground hover:bg-secondary hover:text-foreground aria-expanded:bg-secondary',
                subtle: 'text-subtle-foreground hover:bg-secondary hover:text-foreground aria-expanded:bg-secondary',
                nav: 'text-muted-foreground hover:text-foreground disabled:text-subtle-foreground aria-pressed:text-foreground',
                link: 'underline decoration-border underline-offset-4 hover:text-foreground hover:decoration-current',
                disclosure:
                    'text-muted-foreground hover:bg-foreground-faint hover:text-foreground aria-expanded:bg-foreground-faint aria-expanded:text-foreground [&_svg]:transition-transform [&_svg]:duration-(--motion-duration-ui) [&_svg]:ease-(--ease-out) motion-reduce:[&_svg]:transition-none',
                destructive: `${raised} btn-surface-destructive-tint text-destructive`,
            },
            size: {
                default: 'h-10 px-3.5',
                sm: 'h-8 rounded-sm px-2.5 text-xs',
                text: 'h-8 rounded-sm px-2.5 text-xs -ms-2.5 self-start',
                lg: 'h-11 px-4',
                icon: 'size-10',
                'icon-sm': 'relative size-8 rounded-sm before:absolute before:-inset-1.5',
                'icon-sm-round': 'relative size-8 rounded-full before:absolute before:-inset-1.5',
                'icon-lg': 'size-11',
                avatar: '-me-1 size-10 rounded-full p-1',
                'icon-xs': 'relative size-5 rounded-full before:absolute before:-inset-1',
                pill: '-my-1 gap-1 rounded-full px-2 py-1',
                'pill-start': '-my-1 -ms-2 gap-1 rounded-full px-2 py-1',
                inline: '-my-1 gap-2 rounded-sm px-2 py-1 font-normal',
                bare: '-my-3 rounded-sm py-3 font-normal',
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
        <ButtonPrimitive data-slot="button" className={cn(buttonVariants({ variant, size }), className)} {...props} />
    );
}

export { Button };
