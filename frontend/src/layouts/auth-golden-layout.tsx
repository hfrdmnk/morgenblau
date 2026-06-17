import type { ReactNode } from 'react';

import { AppLogoIcon } from '@/components/app-logo-icon';

export function AuthGoldenLayout({ children }: { children: ReactNode }) {
    return (
        <div className="flex min-h-svh flex-col gap-6 bg-background p-6 sm:flex-row">
            <section className="flex items-center sm:flex-[1.618]">
                <div className="w-full max-w-md pl-6 lg:pl-12">{children}</div>
            </section>
            <div className="flex min-h-[40vh] items-center justify-center overflow-hidden rounded-xl bg-sunrise text-white shadow-[inset_0_0_0_1px_rgba(255,255,255,0.4)] sm:min-h-0 sm:flex-1">
                <AppLogoIcon className="size-16" />
            </div>
        </div>
    );
}
