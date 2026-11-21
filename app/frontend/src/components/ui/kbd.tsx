import type { ComponentProps } from 'react';

// A key the reader can press, drawn as a keycap outline. hideOnTouch drops a hint that a touch screen has no key for.
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
