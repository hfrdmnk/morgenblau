import type { ComponentProps } from 'react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { cn } from '@/lib/utils';

// The group draws the field, so the inner input's focus ring and invalid edge move to the group and wrap the button too.
function InputGroup({ className, ...props }: ComponentProps<'div'>) {
    return (
        <div
            data-slot="input-group"
            className={cn(
                'flex h-10 w-full min-w-0 items-center rounded-md bg-input has-[[data-slot=input]:disabled]:opacity-45 has-[[data-slot=input]:focus-visible]:outline-1 has-[[data-slot=input]:focus-visible]:-outline-offset-1 has-[[data-slot=input]:focus-visible]:outline-ring has-[[aria-invalid=true]]:shadow-[inset_0_0_0_1px_var(--destructive)]',
                className,
            )}
            {...props}
        />
    );
}

function InputGroupInput({ className, ...props }: ComponentProps<typeof Input>) {
    return (
        <Input
            className={cn('h-full flex-1 bg-transparent focus-visible:outline-none aria-invalid:shadow-none disabled:opacity-100', className)}
            {...props}
        />
    );
}

function InputGroupButton({ className, ...props }: ComponentProps<typeof Button>) {
    return (
        <Button
            className={cn('mr-1 text-muted-foreground hover:text-foreground disabled:text-subtle-foreground disabled:opacity-100', className)}
            size="icon-sm"
            type="button"
            variant="ghost"
            {...props}
        />
    );
}

export { InputGroup, InputGroupButton, InputGroupInput };
