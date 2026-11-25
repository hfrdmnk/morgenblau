import { cva } from 'class-variance-authority';
import type { ComponentProps, ReactNode } from 'react';
import { Link as RouterLink } from 'wouter';

import { cn } from 'cn';

function AppShellLayout(props: Omit<ComponentProps<'div'>, 'className' | 'style'>) {
    return <div className="app-shell min-h-dvh bg-background" {...props} />;
}

const banner = cva('app-banner px-5 pb-2 pt-(--banner-top) text-center text-sm font-medium sm:px-8', {
    variants: {
        tone: {
            admin: 'bg-atmosphere-blue text-atmosphere-blue-foreground',
            quiet: 'bg-muted text-muted-foreground',
        },
    },
});

function Banner({ tone, children }: { tone: 'admin' | 'quiet'; children: ReactNode }) {
    return <div className={banner({ tone })}>{children}</div>;
}

function AppHeaderBar(props: Omit<ComponentProps<'header'>, 'className' | 'style'>) {
    return (
        <header
            className="app-header grid grid-cols-(--grid-trailing) items-start gap-y-6 px-5 select-none sm:px-8 md:grid-cols-3 md:px-14"
            {...props}
        />
    );
}

function ModeNav(props: Omit<ComponentProps<'nav'>, 'className' | 'style'>) {
    return <nav className="app-mode-nav select-none" {...props} />;
}

function ModeLink({
    href,
    current,
    artwork,
    children,
    ...props
}: Omit<ComponentProps<'a'>, 'className' | 'style'> & {
    href: string;
    current: boolean;
    artwork: ReactNode;
}) {
    return (
        <RouterLink
            aria-current={current ? 'page' : undefined}
            className={cn('app-mode-link', current ? 'text-foreground' : 'text-muted-foreground hover:text-foreground')}
            href={href}
            {...props}
        >
            <span className={cn('app-mode-artwork', current && 'text-atmosphere-blue')}>{artwork}</span>
            {children}
        </RouterLink>
    );
}

function PageTab({
    href,
    current,
    ...props
}: Omit<ComponentProps<'a'>, 'className' | 'style'> & { href: string; current: boolean }) {
    return (
        <RouterLink
            aria-current={current ? 'page' : undefined}
            className={cn(
                'inline-flex min-h-11 items-center text-sm',
                current ? 'text-foreground' : 'text-muted-foreground hover:text-foreground',
            )}
            href={href}
            {...props}
        />
    );
}

function DayStepper(props: Omit<ComponentProps<'nav'>, 'className' | 'style'>) {
    return (
        <nav
            className="-mx-3.5 grid min-h-11 grid-cols-(--grid-stepper) items-center gap-1 text-sm md:hidden"
            {...props}
        />
    );
}

function DateStrip(props: Omit<ComponentProps<'nav'>, 'className' | 'style'>) {
    return (
        <nav
            className="digest-date-nav hidden grid-cols-7 items-start gap-3.5 font-book md:mt-2.25 md:grid"
            {...props}
        />
    );
}

function DateStripDay({
    href,
    day,
    month,
    selected,
    ...props
}: Omit<ComponentProps<'a'>, 'className' | 'style' | 'children'> & {
    href: string;
    day: string;
    month: string;
    selected: boolean;
}) {
    return (
        <RouterLink
            aria-current={selected ? 'date' : undefined}
            className={cn(
                'flex w-5 flex-col items-center text-sm hover:text-foreground',
                selected ? 'text-primary' : 'text-muted-foreground',
            )}
            href={href}
            {...props}
        >
            <span className="tabular-nums">{day}</span>
            <span className={cn('whitespace-nowrap', { invisible: !selected })}>{month}</span>
        </RouterLink>
    );
}

function DateStripSpacer() {
    return (
        <span aria-hidden="true" className="invisible w-5 text-center text-sm">
            <span className="block">00</span>
            <span className="block">Mon</span>
        </span>
    );
}

export {
    Banner,
    AppHeaderBar,
    AppShellLayout,
    DateStrip,
    DateStripDay,
    DateStripSpacer,
    DayStepper,
    ModeLink,
    ModeNav,
    PageTab,
};
