import type { ReactNode } from 'react';
import { Link } from 'wouter';

import { FeedHealth, GeneratedNote } from '@/components/feed-health';
import { ItemList, ListError, ListItem, ListSkeleton } from '@/components/item-list';
import { useSources, type FeedSource, type NewsletterSource } from '@/hooks/use-sources';
import { newsletterSourceHref, PATHS, sourceHref } from '@/lib/paths';
import { displayURL } from '@/lib/source-display';

export function Sources() {
    return (
        <main aria-label="Sources" className="mx-auto w-full max-w-3xl space-y-14 px-5 pb-20 pt-20 sm:px-8 md:pt-24">
            <h1 className="sr-only">Sources</h1>
            <FeedList />
            <NewsletterList />
        </main>
    );
}

function FeedList() {
    const state = useSources<FeedSource[]>('/api/subscriptions', 15_000);
    return (
        <SourceSection title="Feeds">
            {state.status === 'loaded' ? (
                state.data.length ? (
                    <ItemList>
                        {state.data.map((source) => (
                            <SourceRow key={source.rkey} href={sourceHref(source.rkey)} title={source.title}
                                identity={source.siteUrl || source.feedUrl} favicon={source.faviconUrl}
                                primary={source.primary} health={<FeedHealth source={source} />}
                                note={<GeneratedNote source={source} />} />
                        ))}
                    </ItemList>
                ) : <p className="text-sm text-muted-foreground">No feeds yet. <Link href={PATHS.import} className="underline underline-offset-4">Import feeds</Link> to get started.</p>
            ) : <ListStatus label="feeds" state={state} />}
        </SourceSection>
    );
}

function NewsletterList() {
    const state = useSources<{ active: NewsletterSource[]; stopped: NewsletterSource[] }>('/api/newsletters');
    if (state.status !== 'loaded') {
        return <SourceSection title="Newsletters"><ListStatus label="newsletters" state={state} /></SourceSection>;
    }
    return (
        <>
            <SourceSection title="Newsletters">
                {state.data.active.length ? <NewsletterRows sources={state.data.active} /> :
                    <p className="text-sm text-muted-foreground">Newsletters appear here when mail arrives at your private address.</p>}
            </SourceSection>
            {state.data.stopped.length > 0 ? (
                <SourceSection title="Stopped newsletters">
                    <NewsletterRows sources={state.data.stopped} stopped />
                </SourceSection>
            ) : null}
        </>
    );
}

function SourceSection({ title, children }: { title: string; children: ReactNode }) {
    return (
        <section aria-label={title}>
            <h2 className="text-base font-medium text-muted-foreground">{title}</h2>
            <div className="mt-7">{children}</div>
        </section>
    );
}

function NewsletterRows({ sources, stopped = false }: { sources: NewsletterSource[]; stopped?: boolean }) {
    return (
        <ItemList>
            {sources.map((source) => (
                <SourceRow key={source.id} href={newsletterSourceHref(source.id)} title={source.title}
                    identity={source.senderAddress} primary={!stopped && source.primary} />
            ))}
        </ItemList>
    );
}

function SourceRow({ href, title, identity, favicon, primary, health, note }: {
    href: string; title?: string; identity: string; favicon?: string; primary: boolean; health?: ReactNode; note?: ReactNode;
}) {
    return (
        <ListItem href={href} title={title || displayURL(identity)} icon={favicon}
            aside={primary ? 'Primary' : undefined} note={note}
            meta={displayURL(identity)} status={health} />
    );
}

function ListStatus({ label, state }: { label: string; state: { status: 'loading' | 'error'; retry: () => void } }) {
    return state.status === 'error' ? <ListError label={label} retry={state.retry} /> : <ListSkeleton label={`Loading ${label}`} />;
}
