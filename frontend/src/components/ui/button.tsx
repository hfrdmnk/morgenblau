import { Button as ButtonPrimitive } from '@base-ui/react/button';
import { type VariantProps } from 'class-variance-authority';

import {
    buttonVariants,
    SECONDARY_BY_LEVEL,
    type ButtonVariant,
} from '@/components/ui/button-variants';
import { useSurfaceLevel } from '@/hooks/use-surface-level';
import { cn } from '@/lib/utils';

function Button({
    className,
    variant = 'default',
    size = 'default',
    ...props
}: ButtonPrimitive.Props & VariantProps<typeof buttonVariants>) {
    const level = useSurfaceLevel();
    const variantClass: ButtonVariant = variant ?? 'default';

    return (
        <ButtonPrimitive
            data-slot="button"
            className={cn(
                buttonVariants({ variant: variantClass, size }),
                variantClass === 'secondary' && SECONDARY_BY_LEVEL[level],
                className,
            )}
            {...props}
        />
    );
}

export { Button };
