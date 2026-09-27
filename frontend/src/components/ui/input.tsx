import { Input as InputPrimitive } from '@base-ui/react/input';
import type { ComponentProps } from 'react';

import { cn } from '@/lib/utils';

function Input({ className, type, ...props }: ComponentProps<'input'>) {
    return (
        <InputPrimitive
            type={type}
            data-slot="input"
            className={cn(
                'h-10 w-full min-w-0 rounded-md bg-input px-3 py-2 text-base transition-colors duration-(--motion-duration-fast) placeholder:text-subtle-foreground disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-45 aria-invalid:shadow-[inset_0_0_0_1px_var(--destructive)] md:text-sm',
                className,
            )}
            {...props}
        />
    );
}

export { Input };
