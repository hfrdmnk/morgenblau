import type { ComponentProps } from 'react';

function Kbd({
    hideOnTouch = false,
    ...props
}: Omit<ComponentProps<'kbd'>, 'className' | 'style'> & { hideOnTouch?: boolean }) {
    return (
        <kbd
            className={`rounded-sm px-1.5 font-sans text-xs text-subtle-foreground shadow-hairline${hideOnTouch ? ' hidden pointer-fine:inline' : ''}`}
            {...props}
        />
    );
}

export { Kbd };
