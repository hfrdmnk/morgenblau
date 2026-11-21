import type { ReactNode } from 'react';

// Styleguide helpers: they show tokens and states side by side, which product pages never need.

function ThemeScope({ theme, children }: { theme: 'light' | 'dark'; children: ReactNode }) {
    return <div className={`${theme} text-foreground`}>{children}</div>;
}

function Swatch({ variable }: { variable: string }) {
    return <div className="h-10 rounded-md ring-1 ring-border" style={{ backgroundColor: `var(--${variable})` }} />;
}

const states = {
    hover: '*:from-(--btn-highlight)',
    focus: '*:outline-1 *:outline-offset-1 *:outline-ring',
    'field-focus': '*:-outline-offset-1 *:outline-1 *:outline-ring',
    'field-success': '*:shadow-hairline-success',
};

// Holds its child in a state a reader would otherwise have to hover or focus to see.
function StateSpecimen({ state, children }: { state: keyof typeof states; children: ReactNode }) {
    return <span className={`contents ${states[state]}`}>{children}</span>;
}

function SpecimenAside({ children }: { children: ReactNode }) {
    return <aside className="flex min-w-0 flex-col gap-5 lg:sticky lg:top-8 lg:h-fit">{children}</aside>;
}

// Three page states in one tinted strip, divided where they meet in either direction.
function SpecimenStrip({ children }: { children: ReactNode }) {
    return (
        <div className="grid divide-y divide-border rounded-xl bg-muted lg:grid-cols-3 lg:divide-x lg:divide-y-0">
            {children}
        </div>
    );
}

export { SpecimenAside, SpecimenStrip, StateSpecimen, Swatch, ThemeScope };
