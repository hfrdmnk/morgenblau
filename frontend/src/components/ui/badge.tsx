import { mergeProps } from '@base-ui/react/merge-props';
import { useRender } from '@base-ui/react/use-render';
import { cva, type VariantProps } from 'class-variance-authority';

import { cn } from '@/lib/utils';

const badgeVariants = cva(
    'inline-flex w-fit shrink-0 items-center gap-1 rounded-full px-2 py-1 text-xs font-medium whitespace-nowrap [&>svg]:size-3',
    {
        variants: {
            variant: {
                default: 'bg-secondary text-secondary-foreground',
                success: 'bg-success/10 text-success dark:bg-success/15',
                destructive:
                    'bg-destructive/10 text-destructive dark:bg-destructive/15',
            },
        },
        defaultVariants: {
            variant: 'default',
        },
    },
);

function Badge({
    className,
    variant = 'default',
    render,
    ...props
}: useRender.ComponentProps<'span'> & VariantProps<typeof badgeVariants>) {
    return useRender({
        defaultTagName: 'span',
        props: mergeProps<'span'>(
            {
                className: cn(badgeVariants({ variant }), className),
            },
            props,
        ),
        render,
        state: {
            slot: 'badge',
            variant,
        },
    });
}

export { Badge };
