import { CancelIcon } from '@proicons/react';
import type { ReactNode } from 'react';

import { useGoBackOr } from '@/hooks/use-go-back-or';
import { isPlainLeftClick } from '@/lib/utils';

export function ReaderHeader({ backHref }: { backHref: string }) {
    const goBackOr = useGoBackOr();
    return (
        <header className="sticky top-0 z-10 flex h-14 items-center px-4 sm:px-6">
            <a
                href={backHref}
                aria-label="Back"
                onClick={(e) => {
                    // Keep href for middle-click/new-tab; a plain click prefers history.back() to restore scroll + transition.
                    if (!isPlainLeftClick(e)) return;
                    e.preventDefault();
                    goBackOr(backHref);
                }}
                className="inline-flex size-9 items-center justify-center rounded-xl text-muted-foreground transition-colors duration-200 ease-out outline-none hover:text-foreground focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring focus-visible:outline-solid"
            >
                <CancelIcon className="size-5" />
            </a>
        </header>
    );
}

export function ReaderShell({
    backHref,
    children,
}: {
    backHref: string;
    children: ReactNode;
}) {
    return (
        <div className="min-h-svh bg-card">
            <ReaderHeader backHref={backHref} />
            <article className="mx-auto w-full max-w-2xl px-4 pt-8 pb-24 sm:px-6">
                {children}
            </article>
        </div>
    );
}
