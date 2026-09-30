import { useEffect, useState } from 'react';
import { Link, useParams } from 'wouter';
import { useSearch } from 'wouter/use-browser-location';

import { ArrowRightIcon, ExternalLinkIcon } from '@/components/icons';
import { Button } from '@/components/ui/button';
import { api, ApiError } from '@/lib/api';
import { parseDigestDate, toDateKey } from '@/lib/digest-dates';
import { digestHref, PATHS } from '@/lib/paths';
import { hasNewReaderContent, httpURL, readerBody, readerMetadata, youtubeID, type ReaderEntry } from '@/lib/reader';
import './reader.css';

export function Entry() {
    const { slug = '' } = useParams<{ slug: string }>();
    const search = useSearch();
    const from = parseDigestDate(new URLSearchParams(search).get('from'));

    return (
        <div className="min-h-dvh bg-background">
            <nav aria-label="Reader" className="mx-auto max-w-3xl px-5 pt-6 sm:px-8 sm:pt-10">
                <Link className="inline-flex min-h-10 items-center gap-2 rounded-sm text-sm text-muted-foreground hover:text-foreground" href={digestHref(from ? toDateKey(from) : undefined)}>
                    <ArrowRightIcon className="size-4 rotate-180" />
                    Back to digest
                </Link>
            </nav>
            <main className="mx-auto max-w-3xl px-5 pb-24 pt-12 sm:px-8 sm:pt-16" id="reader">
                <EntryLoader key={slug} slug={slug} />
            </main>
        </div>
    );
}

type EntryState =
    | { status: 'loading' }
    | { status: 'loaded'; entry: ReaderEntry; extracted: boolean; extractionFailed: boolean }
    | { status: 'error'; error: unknown };

function canExtractArticle(entry: ReaderEntry): boolean {
    return entry.contentType === 'blogpost' && Boolean(httpURL(entry.url));
}

async function loadEntry(slug: string, signal: AbortSignal): Promise<Extract<EntryState, { status: 'loaded' }>> {
    const entry = await api<ReaderEntry>(`/api/entries/${encodeURIComponent(slug)}`, { signal });
    const state = { status: 'loaded' as const, entry, extracted: false, extractionFailed: false };
    if (!canExtractArticle(entry) || entry.body?.trim()) return state;
    try {
        state.entry = await api<ReaderEntry>(`/api/entries/${encodeURIComponent(slug)}/extract`, { method: 'POST', signal });
        state.extracted = Boolean(state.entry.body?.trim());
        state.extractionFailed = !state.extracted;
    } catch {
        state.extractionFailed = true;
    }
    return state;
}

function EntryLoader({ slug }: { slug: string }) {
    const [state, setState] = useState<EntryState>({ status: 'loading' });
    const [attempt, setAttempt] = useState(0);

    useEffect(() => {
        const controller = new AbortController();
        const signal = controller.signal;
        void loadEntry(slug, signal)
            .then((next) => { if (!signal.aborted) setState(next); })
            .catch((error: unknown) => {
                if (!signal.aborted) setState({ status: 'error', error });
            });
        return () => controller.abort();
    }, [slug, attempt]);

    if (state.status === 'loading') return <ReaderSkeleton />;
    if (state.status === 'error') {
        const status = state.error instanceof ApiError ? state.error.status : 0;
        return <EntryError status={status} retry={() => { setState({ status: 'loading' }); setAttempt(attempt + 1); }} />;
    }
    return <Reader initialEntry={state.entry} initiallyExtracted={state.extracted} extractionFailed={state.extractionFailed} />;
}

function EntryError({ status, retry }: { status: number; retry: () => void }) {
    const messages: Record<number, [string, string]> = {
        404: ['Entry unavailable', 'It may have been removed, or its source is no longer in your newspaper.'],
        401: ['Sign in to read this entry', 'Your session may have ended.'],
    };
    const [title, description] = messages[status] ?? ['This entry could not be loaded', 'Please try again in a moment.'];
    return (
        <div className="space-y-4" role="alert">
            <h1 className="text-2xl font-medium">{title}</h1>
            <p className="text-muted-foreground">{description}</p>
            {status === 401 ? <Link href={PATHS.login} className="underline">Sign in</Link> : status !== 404 ? <Button variant="secondary" onClick={retry}>Try again</Button> : null}
        </div>
    );
}

function ReaderSkeleton() {
    return (
        <div aria-label="Loading entry" role="status">
            <div aria-hidden="true">
                <div className="mb-4 h-5 w-28 rounded bg-muted" />
                <div className="h-8 w-4/5 rounded bg-muted" />
                <div className="mt-4 h-5 w-44 rounded bg-muted" />
                <div className="mt-5 h-8 w-28 rounded bg-muted" />
                <div className="mt-10 space-y-8">
                    {[0, 1, 2].map((paragraph) => (
                        <div className="space-y-3" key={paragraph}>
                            <div className="h-5 w-full rounded bg-muted" />
                            <div className="h-5 w-full rounded bg-muted" />
                            <div className="h-5 w-3/4 rounded bg-muted" />
                        </div>
                    ))}
                </div>
            </div>
        </div>
    );
}

function Reader({ initialEntry, initiallyExtracted, extractionFailed }: {
    initialEntry: ReaderEntry;
    initiallyExtracted: boolean;
    extractionFailed: boolean;
}) {
    const [entry, setEntry] = useState(initialEntry);
    return (
        <article aria-labelledby="entry-title">
            <ReaderHeader entry={entry} />
            <ReaderMedia entry={entry} onChange={setEntry} />
            <ReaderBody entry={entry} />
            {canExtractArticle(entry) ? <ArticleExtraction entry={entry} onChange={setEntry} initiallyExtracted={initiallyExtracted} extractionFailed={extractionFailed} /> : null}
        </article>
    );
}

function ReaderMedia({ entry, onChange }: { entry: ReaderEntry; onChange: (entry: ReaderEntry) => void }) {
    if (entry.newsletter) return <NewsletterImages entry={entry} newsletter={entry.newsletter} onChange={onChange} />;
    if (entry.contentType === 'video') return <ReaderVideo youtube={youtubeID(entry.url)} video={readerMetadata(entry.metadata).video} title={entry.title || 'Video'} />;
    return null;
}

function ReaderNotice({ notice }: { notice: string }) {
    return notice ? <p className="mt-5 text-sm text-muted-foreground" role="status">{notice}</p> : null;
}

function ArticleExtraction({ entry, onChange, initiallyExtracted, extractionFailed }: {
    entry: ReaderEntry;
    onChange: (entry: ReaderEntry) => void;
    initiallyExtracted: boolean;
    extractionFailed: boolean;
}) {
    const [extracted, setExtracted] = useState(initiallyExtracted);
    const [pending, setPending] = useState(false);
    const [notice, setNotice] = useState(extractionFailed ? 'The full article could not be loaded. You can try again or open the original.' : '');

    async function extract() {
        setPending(true);
        setNotice('');
        try {
            const updated = await api<ReaderEntry>(`/api/entries/${encodeURIComponent(entry.entrySlug)}/extract`, { method: 'POST' });
            if (hasNewReaderContent(entry.body, updated.body)) {
                onChange(updated);
                setExtracted(true);
            } else {
                setNotice('No additional content was found. You can read the original or try again.');
            }
        } catch {
            setNotice('The full article could not be loaded. You can try again or open the original.');
        } finally {
            setPending(false);
        }
    }

    return (
        <>
            {!extracted ? (
                <div className="mt-12 space-y-3 text-sm text-muted-foreground">
                    <p>Only seeing part of the article?</p>
                    <Button disabled={pending} onClick={() => void extract()} variant="secondary">{pending ? 'Loading full article…' : 'Load full article'}</Button>
                </div>
            ) : null}
            <ReaderNotice notice={notice} />
        </>
    );
}

function NewsletterImages({ entry, newsletter, onChange }: {
    entry: ReaderEntry;
    newsletter: NonNullable<ReaderEntry['newsletter']>;
    onChange: (entry: ReaderEntry) => void;
}) {
    const [pending, setPending] = useState(false);
    const [notice, setNotice] = useState('');

    async function allowImages() {
        setPending(true);
        setNotice('');
        try {
            const result = await api<{ body: string; remoteImagesAllowed: boolean; hasBlockedRemoteImages: boolean }>(
                `/api/newsletters/messages/${encodeURIComponent(newsletter.messageId)}/images`, { method: 'POST' },
            );
            onChange({ ...entry, body: result.body, newsletter: { ...newsletter, ...result } });
        } catch {
            setNotice('Images could not be loaded. Please try again.');
        } finally {
            setPending(false);
        }
    }

    return (
        <>
            {newsletter.hasBlockedRemoteImages && !newsletter.remoteImagesAllowed ? (
                <aside className="mb-8 flex flex-wrap items-center justify-between gap-4 rounded-lg bg-muted/60 p-4 text-sm">
                    <div className="max-w-md space-y-1"><p>Remote images are blocked</p><p className="text-muted-foreground">Loading them may tell the sender you opened this message. This choice applies only to this message.</p></div>
                    <Button disabled={pending} onClick={() => void allowImages()} variant="secondary">{pending ? 'Loading images…' : 'Load images'}</Button>
                </aside>
            ) : null}

            <ReaderNotice notice={notice} />
        </>
    );
}

function ReaderBody({ entry }: { entry: ReaderEntry }) {
    if (entry.body?.trim()) return <div className="reader-body" dangerouslySetInnerHTML={{ __html: readerBody(entry) }} />;
    if (entry.contentType === 'video') return null;
    return <p className="text-muted-foreground">{httpURL(entry.url) ? 'There is no readable content here. Open the original to continue.' : 'No content is available for this entry.'}</p>;
}

function sourceTitle(entry: ReaderEntry): string {
    const url = httpURL(entry.source.siteUrl) || httpURL(entry.source.feedUrl);
    return entry.source.title || (url ? new URL(url).hostname : 'Unknown source');
}

function entryTitle(entry: ReaderEntry): string {
    return entry.title || (entry.contentType === 'microblog' ? `Post from ${sourceTitle(entry)}` : 'Untitled');
}

function ReaderHeader({ entry }: { entry: ReaderEntry }) {
    const source = sourceTitle(entry);
    const { author } = readerMetadata(entry.metadata);
    const date = new Date(entry.publishedAt);
    const labels: Record<string, string> = { newsletter: ' · Newsletter', video: ' · Video', microblog: ' · Post' };
    return (
        <header className="mb-10">
            <p className="mb-4 text-sm text-muted-foreground">{source}{labels[entry.contentType]}</p>
            <h1 className="text-3xl font-medium text-balance break-words" id="entry-title">{entryTitle(entry)}</h1>
            <div className="mt-4 flex flex-wrap gap-x-3 gap-y-1 text-sm text-subtle-foreground">
                {author ? <span>{author}</span> : null}
                {!Number.isNaN(date.getTime()) ? <time dateTime={entry.publishedAt}>{date.toLocaleDateString(undefined, { day: 'numeric', month: 'long', year: 'numeric' })}</time> : null}
            </div>
            <OriginalSource entry={entry} />
        </header>
    );
}

function sourceActionLabel(entry: ReaderEntry): string {
    const labels: Record<string, string> = { newsletter: 'Open web version', video: 'Watch at source', microblog: 'View original post' };
    const isYoutube = entry.contentType === 'video' && youtubeID(entry.url);
    return isYoutube ? 'Watch on YouTube' : labels[entry.contentType] ?? 'Read original';
}

function OriginalSource({ entry }: { entry: ReaderEntry }) {
    const original = httpURL(entry.url);
    if (!original) return null;
    return (
        <a className="mt-5 inline-flex min-h-8 items-center gap-2 rounded-sm text-sm text-muted-foreground underline decoration-border underline-offset-4 hover:text-foreground" href={original} target="_blank" rel="noopener noreferrer">
            {sourceActionLabel(entry)}
            <ExternalLinkIcon className="size-3.5" />
        </a>
    );
}

function ReaderVideo({ youtube, video, title }: {
    youtube: string | null;
    video?: { url: string; type: string };
    title: string;
}) {
    if (youtube) return (
        <div className="mb-10 aspect-video overflow-hidden rounded-lg bg-muted">
            <iframe className="h-full w-full border-0" src={`https://www.youtube-nocookie.com/embed/${youtube}`} title={title} allow="accelerometer; autoplay; encrypted-media; gyroscope; picture-in-picture; fullscreen" allowFullScreen referrerPolicy="strict-origin-when-cross-origin" />
        </div>
    );
    if (video) return <video aria-label={title} className="mb-10 aspect-video w-full rounded-lg bg-muted" controls preload="none"><source src={video.url} type={video.type} /><p>Your browser cannot play this video. Open it at the source.</p></video>;
    return <p className="mb-8 text-muted-foreground">This video is available at its original source.</p>;
}
