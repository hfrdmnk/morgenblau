import type { ReactNode } from 'react';

export function DataSettings({ children }: { children: ReactNode }) {
    return (
        <main className="mx-auto w-full max-w-3xl px-5 pb-20 pt-16 sm:px-8 md:pt-24">
            {children}
        </main>
    );
}
