import { useRef, type ReactNode, type RefObject } from 'react';
import { Skeleton } from 'skullet/react';
import { Link } from 'wouter';

import { EditNewsletterSheet, EditSourceSheet } from '@/components/add-source-dialog';
import { FeedHealth, GeneratedNote } from '@/components/feed-health';
import { ItemList, ListError, ListItem } from '@/components/item-list';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { useSources, type FeedSource, type NewsletterSource } from '@/hooks/use-sources';
import { newsletterSourceHref, PATHS, sourceHref } from '@/lib/paths';
import { displayURL, groupByInitial } from '@/lib/source-display';

type RemovedFocus = RefObject<HTMLHeadingElement | null>;

export function Sources() {
    const heading = useRef<HTMLHeadingElement>(null);
    useDocumentTitle('Sources');
    return (
        <main aria-label="Sources" className="mx-auto w-full max-w-3xl space-y-14 px-5 pb-20 pt-20 sm:px-8 md:pt-24" id="main">
            <h1 className="sr-only" ref={heading} tabIndex={-1}>Sources</h1>
            <FeedList removedFocus={heading} />
            <NewsletterList removedFocus={heading} />
        </main>
    );
}

const SAMPLE_FEEDS: FeedSource[] = ['Example Publication', 'Another Example Journal', 'Sample Weekly', 'Example Notes'].map((title, index) => ({
    rkey: `${index}`, title, feedUrl: 'https://example.com/feed.xml', siteUrl: 'https://example.com', primary: index === 0, fetchStatus: 'ready',
}));

const SAMPLE_NEWSLETTERS: NewsletterSource[] = ['Example Newsletter', 'Sample Dispatch'].map((title, index) => ({
    id: `${index}`, title, senderAddress: 'hello@example.com', primary: false, canUnsubscribe: false, tags: [],
}));

function FeedList({ removedFocus }: { removedFocus: RemovedFocus }) {
    const state = useSources<FeedSource[]>('/api/subscriptions', 15_000);
    return (
        <SourceSection title="Feeds">
            <Skeleton
                data={state.status === 'loaded' ? state.data : null}
                error={state.status === 'error' ? <ListError label="feeds" retry={state.retry} /> : undefined}
                id="feeds"
                label="Loading feeds"
                sample={SAMPLE_FEEDS}
            >
                {(feeds) => feeds.length ? (
                    <LetterGroups sources={feeds} label={(source) => sourceLabel(source.title, source.siteUrl || source.feedUrl)}
                        row={(source) => (
                            <SourceRow key={source.rkey} href={sourceHref(source.rkey)} title={source.title}
                                identity={source.siteUrl || source.feedUrl} favicon={source.faviconUrl}
                                primary={source.primary} health={<FeedHealth labelled source={source} />}
                                note={<GeneratedNote source={source} />}
                                menu={<EditSourceSheet onChanged={state.refresh} removedFocus={removedFocus} source={source} />} />
                        )} />
                ) : <p className="text-sm text-muted-foreground">No feeds yet. <Link href={PATHS.import} className="underline underline-offset-4">Import feeds</Link> to get started.</p>}
            </Skeleton>
        </SourceSection>
    );
}

function NewsletterList({ removedFocus }: { removedFocus: RemovedFocus }) {
    const state = useSources<{ active: NewsletterSource[]; stopped: NewsletterSource[] }>('/api/newsletters');
    return (
        <>
            <SourceSection title="Newsletters">
                <Skeleton
                    data={state.status === 'loaded' ? state.data.active : null}
                    error={state.status === 'error' ? <ListError label="newsletters" retry={state.retry} /> : undefined}
                    id="newsletters"
                    label="Loading newsletters"
                    sample={SAMPLE_NEWSLETTERS}
                >
                    {(active) => active.length ? <NewsletterRows onChanged={state.refresh} removedFocus={removedFocus} sources={active} /> :
                        <p className="text-sm text-muted-foreground">Newsletters appear here when mail arrives at your private address.</p>}
                </Skeleton>
            </SourceSection>
            {state.status === 'loaded' ? <StoppedNewsletters onChanged={state.refresh} removedFocus={removedFocus} sources={state.data.stopped} /> : null}
        </>
    );
}

function StoppedNewsletters({ sources, onChanged, removedFocus }: { sources: NewsletterSource[]; onChanged: () => void; removedFocus: RemovedFocus }) {
    if (!sources.length) return null;
    return (
        <SourceSection title="Stopped newsletters">
            <NewsletterRows onChanged={onChanged} removedFocus={removedFocus} sources={sources} stopped />
        </SourceSection>
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

function NewsletterRows({ sources, stopped = false, onChanged, removedFocus }: {
    sources: NewsletterSource[]; stopped?: boolean; onChanged: () => void; removedFocus: RemovedFocus;
}) {
    return (
        <LetterGroups sources={sources} label={(source) => sourceLabel(source.title, source.senderAddress)}
            row={(source) => (
                <SourceRow key={source.id} href={newsletterSourceHref(source.id)} title={source.title}
                    identity={source.senderAddress} primary={!stopped && source.primary}
                    menu={<EditNewsletterSheet onChanged={onChanged} removedFocus={removedFocus} source={source} stopped={stopped} />} />
            )} />
    );
}

function LetterGroups<T>({ sources, label, row }: { sources: T[]; label: (source: T) => string; row: (source: T) => ReactNode }) {
    return (
        <div className="space-y-12">
            {groupByInitial(sources, label).map(({ initial, items }) => (
                <div key={initial}>
                    <h3 className="text-sm font-medium text-subtle-foreground">{initial}</h3>
                    <div className="mt-5"><ItemList>{items.map(row)}</ItemList></div>
                </div>
            ))}
        </div>
    );
}

function sourceLabel(title: string | undefined, identity: string): string {
    return title || displayURL(identity);
}

function SourceRow({ href, title, identity, favicon, primary, health, note, menu }: {
    href: string; title?: string; identity: string; favicon?: string; primary: boolean; health?: ReactNode; note?: ReactNode; menu: ReactNode;
}) {
    return (
        <ListItem href={href} title={sourceLabel(title, identity)} icon={favicon}
            aside={primary ? 'Primary' : undefined} note={note}
            meta={displayURL(identity)} status={health} actions={menu} />
    );
}
