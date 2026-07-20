import type { ReactNode } from 'react';

import { CardMastheadSkeleton } from '@/components/card-masthead';

// Loading frame shared by the discover panels; `row` is one placeholder row's content.
export function DiscoverStackSkeleton({
    label,
    row,
}: {
    label: string;
    row: ReactNode;
}) {
    return (
        <article
            aria-busy
            aria-label={label}
            className="overflow-hidden rounded-xl bg-card shadow-card"
        >
            <CardMastheadSkeleton />
            <div aria-hidden className="mx-6 border-t border-border" />
            {Array.from({ length: 4 }).map((_, index) => (
                <div key={index}>
                    {index > 0 ? (
                        <div aria-hidden className="mx-6 border-t border-border" />
                    ) : null}
                    <div className="px-6 py-5">{row}</div>
                </div>
            ))}
        </article>
    );
}
