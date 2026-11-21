import type { ComponentProps, ReactNode } from 'react';

import { cn } from 'cn';

type RadioCardProps = Omit<ComponentProps<'input'>, 'className' | 'style' | 'type' | 'children'> & { children: ReactNode };

// The radio stays in the tab order but out of sight, so the card draws its focus ring and, in forced colours, its checked border.
function RadioCard({ children, checked, ...props }: RadioCardProps) {
    return (
        <label data-slot="radio-card" className="relative cursor-pointer select-none">
            <input type="radio" checked={checked} className="peer sr-only" {...props} />
            <span
                className={cn(
                    'flex flex-col gap-5 rounded-xl p-5 peer-focus-visible:outline-2 peer-focus-visible:outline-offset-4 peer-focus-visible:outline-ring forced-colors:border-2 forced-colors:border-transparent forced-colors:peer-checked:border-system-highlight',
                    checked ? 'bg-accent text-accent-foreground' : 'bg-muted text-foreground',
                )}
            >
                {children}
            </span>
        </label>
    );
}

export { RadioCard };
