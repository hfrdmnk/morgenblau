import type { ReactNode } from 'react';
import { Link } from 'wouter';

import { PATHS } from '@/lib/paths';
import { cn } from '@/lib/utils';

export function DataSettings({
    page,
    children,
}: {
    page: 'import' | 'export';
    children: ReactNode;
}) {
    return (
        <>
            <nav
                aria-label="Data settings"
                className="flex justify-center gap-6 px-5 pt-12 text-sm sm:px-8"
            >
                {(['import', 'export'] as const).map((item) => (
                    <Link
                        key={item}
                        href={PATHS[item]}
                        aria-current={page === item ? 'page' : undefined}
                        className={cn(
                            'rounded-sm py-1',
                            page === item
                                ? 'font-medium text-foreground'
                                : 'text-subtle-foreground hover:text-foreground',
                        )}
                    >
                        {item === 'import' ? 'Import' : 'Export'}
                    </Link>
                ))}
            </nav>
            <main className="mx-auto w-full max-w-3xl px-5 pb-20 pt-16 sm:px-8 md:pt-24">
                {children}
            </main>
        </>
    );
}
