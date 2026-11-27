import type { ComponentPropsWithRef } from 'react';

import { cn } from 'cn';

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
