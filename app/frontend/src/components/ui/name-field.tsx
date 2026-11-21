import type { ComponentProps } from 'react';

import { PenIcon } from '@/components/icons';
import { cn } from 'cn';

// A name edited in place, as a heading: the field sizes to its text, and an underline draws in on focus, or stays while invalid.
// The label spans the name and its pen, so a click anywhere on either edits the name.
function NameField({
    invalid = false,
    disabled,
    ...props
}: Omit<ComponentProps<'input'>, 'className' | 'style' | 'aria-invalid'> & { invalid?: boolean }) {
    return (
        <label
            className={cn(
                'group flex h-9 max-w-full min-w-0 cursor-text items-center gap-1.5',
                disabled && 'pointer-events-none',
            )}
        >
            <span className="relative flex min-w-0">
                <input
                    aria-invalid={invalid}
                    autoComplete="off"
                    className="min-w-0 truncate bg-transparent text-base font-medium outline-none field-sizing-content"
                    data-slot="name-field"
                    disabled={disabled}
                    spellCheck={false}
                    {...props}
                />
                <span
                    aria-hidden="true"
                    className={cn(
                        'absolute inset-x-0 bottom-0 h-px origin-left transition-transform duration-(--motion-duration-ui) ease-(--ease-out) group-focus-within:scale-x-100',
                        invalid ? 'scale-x-100 bg-destructive' : 'scale-x-0 bg-subtle-line',
                    )}
                />
            </span>
            <PenIcon aria-hidden="true" size="sm" tone="subtle" />
        </label>
    );
}

export { NameField };
