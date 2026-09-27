import type { ComponentProps } from 'react';

import { cn } from '@/lib/utils';

function Link({ className, ...props }: ComponentProps<'a'>) {
    return (
        <a
            data-slot="link"
            className={cn(
                'inline-flex w-fit items-center gap-1 font-medium underline decoration-current/35 underline-offset-3 transition-[color,text-decoration-color] duration-(--motion-duration-fast) hover:decoration-current',
                className,
            )}
            {...props}
        />
    );
}

export { Link };
