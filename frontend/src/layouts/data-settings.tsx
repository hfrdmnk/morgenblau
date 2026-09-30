import type { ReactNode } from 'react';
import { Link } from 'wouter';

import { AccountMenu, AccountMenuSkeleton } from '@/components/account-menu';
import { DigestIcon } from '@/components/icons';
import { useAppProfile } from '@/hooks/use-app-profile';
import { AppShell } from '@/layouts/app-shell';
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
        <AppShell>
            <DataSettingsContent page={page}>{children}</DataSettingsContent>
        </AppShell>
    );
}

function DataSettingsContent({
    page,
    children,
}: {
    page: 'import' | 'export';
    children: ReactNode;
}) {
    const profile = useAppProfile();

    return (
        <div className="min-h-dvh bg-background">
            <header className="grid grid-cols-[1fr_auto] items-center gap-y-6 px-5 pt-6 sm:px-8 sm:pt-8 md:grid-cols-3 md:px-14 md:pt-10">
                <Link
                    href={PATHS.digest}
                    className="inline-flex items-center gap-3 rounded-md text-sm text-muted-foreground"
                >
                    <DigestIcon className="size-8 text-atmosphere-blue" />
                    Digest
                </Link>
                <div className="col-start-2 row-start-1 flex justify-self-end md:col-start-3">
                    {profile.kind === 'ready' ? (
                        <AccountMenu profile={profile.profile} />
                    ) : (
                        <AccountMenuSkeleton />
                    )}
                </div>
                <nav
                    aria-label="Data settings"
                    className="col-span-2 row-start-2 flex justify-center gap-6 text-sm md:col-span-1 md:col-start-2 md:row-start-1"
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
            </header>
            <main className="mx-auto w-full max-w-3xl px-5 pb-20 pt-16 sm:px-8 md:pt-24">
                {children}
            </main>
        </div>
    );
}
