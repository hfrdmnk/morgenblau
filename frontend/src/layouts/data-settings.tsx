import type { ReactNode } from 'react';
import { Link } from 'wouter';

import { ArrowRightIcon } from '@/components/icons';
import type { AppNavigationState } from '@/hooks/use-app-location';
import { appMode, PATHS } from '@/lib/paths';

export function DataSettings({ children }: { children: ReactNode }) {
    const state: AppNavigationState | null = window.history.state;
    const returnTo = state?.settingsReturn ?? { href: PATHS.digest, scrollY: 0 };
    const label = appMode(returnTo.href.split('?')[0])?.label ?? 'Digest';
    return (
        <main className="mx-auto w-full max-w-3xl px-5 pb-20 pt-16 sm:px-8 md:pt-24">
            <Link
                href={returnTo.href}
                state={{ scrollY: returnTo.scrollY }}
                className="mb-6 inline-flex min-h-11 items-center gap-2 text-sm text-muted-foreground hover:text-foreground"
            >
                <ArrowRightIcon className="size-4 rotate-180" />
                Back to {label}
            </Link>
            {children}
        </main>
    );
}
