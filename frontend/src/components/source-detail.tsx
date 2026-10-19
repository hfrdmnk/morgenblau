import type { ReactNode } from 'react';

import { BackLink } from '@/components/back-link';
import { ItemList, ListError, ListItem, ListSkeleton, SourceIcon } from '@/components/item-list';
import { useSources, type SourceEntry } from '@/hooks/use-sources';
import { entryHref, PATHS } from '@/lib/paths';
import { formatDate } from '@/lib/source-display';

type Loadable<T> = ReturnType<typeof useSources<T>>;

export function SourceDetailPage({ children }: { children: ReactNode }) {
    return (
        <main className="mx-auto w-full max-w-3xl px-5 pb-20 pt-6 sm:px-8 md:pt-10">
            <nav aria-label="Source">
                <BackLink href={PATHS.sources} label="Sources" />
            </nav>
            <div className="pt-10 md:pt-12">{children}</div>
        </main>
    );
}

export function SourceHeader({ title, icon, identity, facts, tags = [], health, menu }: {
    title: string;
    icon?: string | null;
    identity?: ReactNode;
    facts: string[];
    tags?: string[];
    health?: ReactNode;
    menu: ReactNode;
}) {
    return (
        <header className="mb-14">
            <div className="flex min-w-0 items-center gap-3">
                <SourceIcon className="size-6" src={icon} />
                <h1 className="min-w-0 flex-1 text-3xl font-medium text-balance break-words">{title}</h1>
                {menu}
            </div>
            {identity ? <div className="mt-3 min-w-0 text-sm text-muted-foreground">{identity}</div> : null}
            <p className="mt-1 text-sm text-subtle-foreground">{facts.join(' · ')}</p>
            {health}
            <SourceTags tags={tags} />
        </header>
    );
}

function SourceTags({ tags }: { tags: string[] }) {
    if (!tags.length) return null;
    return (
        <ul aria-label="Tags" className="mt-4 flex flex-wrap gap-2">
            {tags.map((tag) => <li className="rounded-full bg-muted px-2.5 py-0.5 text-xs text-muted-foreground" key={tag}>{tag}</li>)}
        </ul>
    );
}

export function SourceEntries({ title, ...props }: {
    title: string;
    from: string;
    state: Loadable<SourceEntry[]>;
    empty: string;
}) {
    return (
        <section aria-label={title}>
            <h2 className="text-base font-medium text-muted-foreground">{title}</h2>
            <div className="mt-7">
                <EntriesBody label={title.toLowerCase()} {...props} />
            </div>
        </section>
    );
}

function EntriesBody({ label, from, state, empty }: {
    label: string;
    from: string;
    state: Loadable<SourceEntry[]>;
    empty: string;
}) {
    if (state.status === 'loading') return <ListSkeleton label={`Loading ${label}`} />;
    if (state.status === 'error') return <ListError label={label} retry={state.retry} />;
    if (!state.data.length) return <p className="text-sm text-muted-foreground">{empty}</p>;
    return (
        <ItemList>
            {state.data.map((entry) => (
                <ListItem
                    href={entryHref(entry.entrySlug, from)}
                    key={entry.entrySlug}
                    meta={formatDate(entry.publishedAt)}
                    title={entry.title || 'Untitled'}
                />
            ))}
        </ItemList>
    );
}

export function SourceUnavailable<T>({ state, label }: { state: Exclude<Loadable<T>, { status: 'loaded' }>; label: string }) {
    if (state.status === 'loading') {
        return (
            <div aria-label={`Loading ${label}`} role="status">
                <div className="h-9 w-3/5 animate-pulse rounded bg-muted motion-reduce:animate-none" />
                <div className="mt-3 h-5 w-48 animate-pulse rounded bg-muted motion-reduce:animate-none" />
            </div>
        );
    }
    if (state.notFound) {
        return (
            <div className="space-y-3" role="alert">
                <h1 className="text-2xl font-medium">Source unavailable</h1>
                <p className="text-muted-foreground">It may have been removed from your sources.</p>
            </div>
        );
    }
    return <ListError label={label} retry={state.retry} />;
}
