import type { ReactNode } from 'react';
import { Link } from 'wouter';

import { Button } from '@/components/ui/button';
import { useSources, type FeedSource, type NewsletterSource } from '@/hooks/use-sources';
import { PATHS } from '@/lib/paths';

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
    const state = useSources<FeedSource[]>('/api/subscriptions');
    return (
        <SourceSection title="Feeds">
            {state.status === 'loaded' ? (
                state.data.length ? (
                    <ul className="space-y-8">
                        {state.data.map((source) => (
                            <SourceRow key={source.rkey} title={source.title}
                                identity={source.siteUrl || source.feedUrl} favicon={source.faviconUrl}
                                primary={source.primary}>
                                {source.muted ? <p className="mt-2 text-sm text-subtle-foreground">Updates unavailable. Retrying automatically.</p> : null}
                            </SourceRow>
                        ))}
                    </ul>
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
        <ul className="space-y-8">
            {sources.map((source) => (
                <SourceRow key={source.id} title={source.title} identity={source.senderAddress}
                    primary={!stopped && source.primary} />
            ))}
        </ul>
    );
}

function SourceRow({ title, identity, favicon, primary, children }: {
    title?: string; identity: string; favicon?: string; primary: boolean; children?: ReactNode;
}) {
    return (
        <li>
            <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
                <h3 className="min-w-0 break-words text-xl font-medium [overflow-wrap:anywhere]">{title || identity}</h3>
                {primary ? <span className="text-xs text-muted-foreground">Primary</span> : null}
            </div>
            <div className="mt-2 flex items-center gap-2.5 text-sm text-subtle-foreground">
                {favicon ? <img alt="" src={favicon} loading="lazy" referrerPolicy="no-referrer" className="size-4 shrink-0 rounded-sm object-cover" onError={(event) => { event.currentTarget.hidden = true; }} /> : null}
                <span className="min-w-0 break-all">{identity.replace(/^https?:\/\//, '').replace(/\/$/, '')}</span>
            </div>
            {children}
        </li>
    );
}

function ListStatus({ label, state }: { label: string; state: { status: 'loading' | 'error'; retry: () => void } }) {
    if (state.status === 'error') return (
        <div role="alert" className="flex flex-wrap items-center gap-4 text-sm text-muted-foreground">
            <p>Couldn’t load {label}.</p>
            <Button onClick={state.retry} variant="secondary" size="sm">Try again</Button>
        </div>
    );
    return <div role="status" aria-label={`Loading ${label}`} className="space-y-6">{[64, 48].map((width) => <div key={width} className="space-y-3"><div className="h-6 rounded bg-muted" style={{ width: `${width}%` }} /><div className="h-4 w-1/3 rounded bg-muted" /></div>)}</div>;
}
