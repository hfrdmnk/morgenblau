import type { ComponentProps } from 'react';

import { cn } from '@/lib/utils';

// Borderless overlay field, matching Input: overlay-1 at rest, overlay-2 on
// focus, no border, no ring. field-sizing-content lets it grow with the text.
function Textarea({ className, ...props }: ComponentProps<'textarea'>) {
    return (
        <textarea
            data-slot="textarea"
            className={cn(
                'field-sizing-content min-h-20 w-full min-w-0 resize-none rounded-xl bg-overlay-1 px-2.5 py-2 text-base transition-colors outline-none placeholder:text-muted-foreground focus-visible:bg-overlay-2 disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:ring-1 aria-invalid:ring-destructive md:text-sm',
                className,
            )}
            {...props}
        />
    );
}

export { Textarea };
