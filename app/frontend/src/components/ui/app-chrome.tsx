import type { ComponentProps, ReactNode } from 'react';
import { Link as RouterLink } from 'wouter';

import { cn } from 'cn';

// The app's frame: the classes named here live in src/index.css, which owns their sticky and fixed placement and safe-area math.

// Block flow, not a flex column: `.app-shell > main` centres the page with auto margins, which a flex column would shrink to fit.
function AppShellLayout(props: Omit<ComponentProps<'div'>, 'className' | 'style'>) {
    return <div className="app-shell min-h-dvh bg-background" {...props} />;
}

function AdminBanner({ children }: { children: ReactNode }) {
    return (
        <div className="admin-banner bg-atmosphere-blue px-5 pb-2 pt-(--admin-banner-top) text-center text-sm font-medium text-atmosphere-blue-foreground sm:px-8">
            {children}
        </div>
    );
}

// A brand-mark or back link on the left, the page's own navigation in the middle, account controls on the right.
function AppHeaderBar(props: Omit<ComponentProps<'header'>, 'className' | 'style'>) {
    return <header className="app-header grid grid-cols-(--grid-trailing) items-start gap-y-6 px-5 select-none sm:px-8 md:grid-cols-3 md:px-14" {...props} />;
}

function ModeNav(props: Omit<ComponentProps<'nav'>, 'className' | 'style'>) {
    return <nav className="app-mode-nav select-none" {...props} />;
}

// The artwork shows only in the phone tab bar; the current mode's takes the brand blue.
function ModeLink({ href, current, artwork, children, ...props }: Omit<ComponentProps<'a'>, 'className' | 'style'> & {
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

// A touch-sized tab on the phone, where the tabs sit in their own row; on wider screens the row's gap spaces them.
function SettingsTab({ href, current, ...props }: Omit<ComponentProps<'a'>, 'className' | 'style'> & { href: string; current: boolean }) {
    return (
        <RouterLink
            aria-current={current ? 'page' : undefined}
            className={cn('inline-flex min-h-11 items-center px-3.5 text-sm md:px-0', current ? 'text-foreground' : 'text-muted-foreground hover:text-foreground')}
            href={href}
            {...props}
        />
    );
}

// The phone's day control: an arrow either side of the date, each a 44px touch target.
function DayStepper(props: Omit<ComponentProps<'nav'>, 'className' | 'style'>) {
    return <nav className="grid min-h-11 grid-cols-(--grid-stepper) items-center gap-1 text-sm md:hidden" {...props} />;
}

// The desktop row of days; the selected one sits in the centre column, which the mask in `.digest-date-nav` relies on.
function DateStrip(props: Omit<ComponentProps<'nav'>, 'className' | 'style'>) {
    return <nav className="digest-date-nav hidden grid-cols-7 items-start gap-3.5 font-book md:mt-2.25 md:grid" {...props} />;
}

// Each column is as wide as two digits; a flex column centres the month even where it is wider than that.
function DateStripDay({ href, day, month, selected, ...props }: Omit<ComponentProps<'a'>, 'className' | 'style' | 'children'> & {
    href: string;
    day: string;
    month: string;
    selected: boolean;
}) {
    return (
        <RouterLink
            aria-current={selected ? 'date' : undefined}
            className={cn('flex w-5 flex-col items-center text-sm hover:text-foreground', selected ? 'text-primary' : 'text-muted-foreground')}
            href={href}
            {...props}
        >
            <span className="tabular-nums">{day}</span>
            <span className={cn('whitespace-nowrap', { invisible: !selected })}>{month}</span>
        </RouterLink>
    );
}

// Holds a day's column before the strip reaches today, so the selected day stays in the centre.
function DateStripSpacer() {
    return (
        <span aria-hidden="true" className="invisible w-5 text-center text-sm">
            <span className="block">00</span>
            <span className="block">Mon</span>
        </span>
    );
}

function PagePlaceholder(props: Omit<ComponentProps<'main'>, 'className' | 'style'>) {
    return <main className="page-placeholder" {...props} />;
}

export { AdminBanner, AppHeaderBar, AppShellLayout, DateStrip, DateStripDay, DateStripSpacer, DayStepper, ModeLink, ModeNav, PagePlaceholder, SettingsTab };
