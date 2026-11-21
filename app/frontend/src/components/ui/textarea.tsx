import type { ComponentProps } from 'react';

import { cn } from 'cn';

function Textarea({ className, ...props }: ComponentProps<'textarea'>) {
    return (
        <textarea
            data-slot="textarea"
            className={cn(
                'field-sizing-content min-h-24 w-full resize-y rounded-md bg-input px-3 py-2 text-base placeholder:text-subtle-foreground disabled:cursor-not-allowed disabled:opacity-45 aria-invalid:shadow-hairline-destructive md:text-sm',
                className,
            )}
            {...props}
        />
    );
}

export { Textarea };
