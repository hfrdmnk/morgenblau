import type { ComponentProps } from 'react';
import type { VariantProps } from 'class-variance-authority';

import { badgeVariants } from '@/components/ui/badge-variants';
import { cn } from '@/lib/utils';

function Badge({
    className,
    variant,
    ...props
}: ComponentProps<'span'> & VariantProps<typeof badgeVariants>) {
    return (
        <span
            data-slot="badge"
            className={cn(badgeVariants({ variant }), className)}
            {...props}
        />
    );
}

export { Badge };
