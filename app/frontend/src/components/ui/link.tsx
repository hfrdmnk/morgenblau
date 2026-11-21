import { cva, type VariantProps } from 'class-variance-authority';
import type { ComponentProps } from 'react';
import { Link as RouterLink } from 'wouter';

const linkVariants = cva('', {
    variants: {
        variant: {
            inline: 'inline-flex w-fit items-center gap-1 font-medium underline decoration-underline underline-offset-3 transition-[color,text-decoration-color] duration-(--motion-duration-fast) hover:decoration-current',
            underline:
                'underline decoration-border underline-offset-4 transition-[color,text-decoration-color] duration-(--motion-duration-fast) hover:text-foreground hover:decoration-current',
            // An underlined link with a trailing icon, tall enough to tap on its own line.
            external:
                'inline-flex min-h-8 items-center gap-2 rounded-sm underline decoration-border underline-offset-4 transition-[color,text-decoration-color] duration-(--motion-duration-fast) hover:text-foreground hover:decoration-current',
            title: 'rounded-sm text-foreground transition-[color] duration-(--motion-duration-fast) hover:text-foreground-soft',
            // A title link laid out as a row, such as a favicon beside a source's name, whose label can truncate.
            'title-row':
                'flex min-w-0 items-center gap-2 rounded-sm text-foreground transition-[color] duration-(--motion-duration-fast) hover:text-foreground-soft',
            quiet: 'rounded-sm transition-[color] duration-(--motion-duration-fast) hover:text-foreground',
            muted: 'rounded-sm text-muted-foreground transition-[color] duration-(--motion-duration-fast) hover:text-foreground',
            // The Morgenblau mark leading home, as tall as a Button so it lines up with the controls beside it.
            brand: 'flex min-h-10 w-fit items-center text-muted-foreground transition-[color] duration-(--motion-duration-fast) hover:text-foreground',
            // Arrow and label back to where the reader came from, at the 44px touch height.
            back: 'inline-flex min-h-11 w-fit items-center gap-2 rounded-sm text-sm text-muted-foreground transition-[color] duration-(--motion-duration-fast) select-none hover:text-foreground',
            // An arrow that steps to the previous or next item, sized to the 44px touch target.
            step: 'flex size-11 items-center justify-center text-muted-foreground transition-[color] duration-(--motion-duration-fast) hover:text-foreground',
            skip: 'sr-only rounded-md bg-popover px-4 py-2 text-sm font-medium shadow-popover select-none focus:not-sr-only focus:fixed focus:top-(--skip-link-top) focus:left-4 focus:z-50',
            plain: '',
        },
    },
    defaultVariants: { variant: 'inline' },
});

type LinkProps = Omit<ComponentProps<'a'>, 'className' | 'style'> & { href: string; state?: unknown } & VariantProps<
        typeof linkVariants
    >;

// App paths navigate in place through the router; anything else, such as an external page, a #fragment or a link that opens
// another tab, is a plain anchor: wouter navigates in place on click and ignores target.
function Link({ variant, state, ...props }: LinkProps) {
    const className = linkVariants({ variant }) || undefined;
    if (props.href.startsWith('/') && !props.href.startsWith('//') && !props.target)
        return <RouterLink data-slot="link" className={className} state={state} {...props} />;
    return <a data-slot="link" className={className} {...props} />;
}

export { Link };
