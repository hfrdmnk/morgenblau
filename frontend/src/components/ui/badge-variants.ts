import { cva } from 'class-variance-authority';

export const badgeVariants = cva(
    "inline-flex w-fit shrink-0 items-center gap-1 rounded-lg px-2 py-1 text-xs font-medium whitespace-nowrap text-foreground/80 [&>svg]:pointer-events-none [&>svg:not([class*='size-'])]:size-3",
    {
        variants: {
            variant: {
                default: 'bg-overlay-2',
                destructive: 'bg-destructive/10 text-destructive',
            },
        },
        defaultVariants: {
            variant: 'default',
        },
    },
);
