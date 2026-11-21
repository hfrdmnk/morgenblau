import type { ComponentPropsWithRef } from 'react';

import { cn } from 'cn';

// The line under a form field. Scrolled into view, it keeps clear of the sheet's bottom padding instead of sitting on its edge.
function FieldMessage({
    invalid = false,
    ...props
}: Omit<ComponentPropsWithRef<'p'>, 'className' | 'style'> & { invalid?: boolean }) {
    return (
        <p
            data-slot="field-message"
            className={cn('scroll-mb-6 text-sm sm:scroll-mb-8', invalid ? 'text-destructive' : 'text-muted-foreground')}
            {...props}
        />
    );
}

export { FieldMessage };
