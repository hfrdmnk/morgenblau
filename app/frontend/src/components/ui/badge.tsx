import { mergeProps } from '@base-ui/react/merge-props';
import { useRender } from '@base-ui/react/use-render';
import { cva, type VariantProps } from 'class-variance-authority';

import { cn } from 'cn';

const badgeVariants = cva(
    'inline-flex w-fit shrink-0 items-center gap-1 rounded-full px-2 py-1 text-xs font-medium whitespace-nowrap [&>svg]:size-3',
    {
        variants: {
            variant: {
                default: 'bg-secondary text-secondary-foreground',
                tag: 'bg-muted text-muted-foreground',
                success: 'bg-success-soft text-success',
                destructive: 'bg-destructive-soft text-destructive',
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
