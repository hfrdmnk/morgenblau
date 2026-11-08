import { useEffect, useState } from 'react';
import { Skeleton } from 'skullet/react';
import { Link, useLocation, useParams } from 'wouter';
import { useSearch } from 'wouter/use-browser-location';

import { BackLink } from '@/components/back-link';
import { NewsletterImages } from '@/components/newsletter-images';
import { ReaderNotice } from '@/components/reader-notice';
import { ReaderRail } from '@/components/reader-rail';
import { SaveFiling } from '@/components/save-filing';
import { Button } from '@/components/ui/button';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { useCommands } from '@/hooks/use-commands';
import { useEntrySave, useSaveState } from '@/hooks/use-entry-save';
import { useEntrySteps } from '@/hooks/use-entry-steps';
import { useReaderFont } from '@/hooks/use-reader-font';
import { useModeShortcuts, useShortcuts } from '@/hooks/use-shortcuts';
import { api, ApiError } from '@/lib/api';
import { CommandProblem, copied, openInTab, type Command } from '@/lib/commands';
import { parseDigestDate, toDateKey } from '@/lib/digest-dates';
import { digestHref, PATHS, readerSourceHref, sourceOrigin } from '@/lib/paths';
import type { EntrySave } from '@/lib/entry-save';
import { allowRemoteImages, hasNewReaderContent, httpURL, readerBody, readerMetadata, sourceActionLabel, youtubeID, type ReaderEntry } from '@/lib/reader';
import { announceRemove, announceSave } from '@/lib/save-announce';
import './reader.css';

export function Entry() {
    const { slug = '' } = useParams<{ slug: string }>();
    const from = new URLSearchParams(useSearch()).get('from');
    const source = sourceOrigin(from);
    const date = parseDigestDate(from);
    const back = source ?? digestHref(date ? toDateKey(date) : undefined);
    const backLabel = source ? 'Back to source' : 'Back to digest';
    const [, navigate] = useLocation();
    useModeShortcuts();
    const steps = useEntrySteps(slug, from);
    useShortcuts({ Escape: () => navigate(back) });
    // The reader registers these after its own actions, so saving leads the menu and Enter never leaves the entry by surprise.
    const navigation: Command[] = [
        { id: 'entry:previous', group: READER, label: 'Previous entry', run: steps.previous },
        { id: 'entry:next', group: READER, label: 'Next entry', run: steps.next },
        { id: 'entry:back', group: READER, label: backLabel, run: () => navigate(back) },
    ];

    return (
        <div className="min-h-dvh bg-background">
            <nav aria-label="Reader" className="mx-auto max-w-3xl px-5 pt-6 sm:px-8 sm:pt-10">
                <BackLink href={back} keyShortcut="Escape" label={backLabel} />
            </nav>
            <main className="mx-auto max-w-3xl px-5 pb-24 pt-12 sm:px-8 sm:pt-16" id="reader">
                <EntryLoader key={slug} navigation={navigation} slug={slug} />
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

function EntryLoader({ slug, navigation }: { slug: string; navigation: Command[] }) {
    const [state, setState] = useState<EntryState>({ status: 'loading' });
    const [attempt, setAttempt] = useState(0);
    useDocumentTitle(readerTitle(state));

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

    const error = state.status === 'error' ? (
        <EntryError
            status={state.error instanceof ApiError ? state.error.status : 0}
            retry={() => { setState({ status: 'loading' }); setAttempt(attempt + 1); }}
        />
    ) : undefined;
    return (
        <Skeleton data={state.status === 'loaded' ? state : null} error={error} id="entry" label="Loading entry" sample={SAMPLE_ENTRY}>
            {(loaded) => <Reader initialEntry={loaded.entry} initiallyExtracted={loaded.extracted} extractionFailed={loaded.extractionFailed} live={loaded !== SAMPLE_ENTRY} navigation={navigation} />}
        </Skeleton>
    );
}

const SAMPLE_PARAGRAPH = 'A paragraph of sample text that stands in for the entry while it loads. It runs across several lines, about as long as a paragraph in a typical post, so the skeleton fills the column the way the article will.';

const SAMPLE_ENTRY: Extract<EntryState, { status: 'loaded' }> = {
    status: 'loaded',
    extracted: true,
    extractionFailed: false,
    entry: {
        entrySlug: 'sample',
        title: 'A sample headline about as long as most entries have',
        contentType: 'blogpost',
        publishedAt: '2026-01-01T00:00:00Z',
        body: `<p>${SAMPLE_PARAGRAPH}</p>`.repeat(3),
        source: { title: 'Example Publication', siteUrl: 'https://example.com' },
    },
};

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

// live is false for the loading sample, which must not offer its commands.
function Reader({ initialEntry, initiallyExtracted, extractionFailed, live, navigation }: {
    initialEntry: ReaderEntry;
    initiallyExtracted: boolean;
    extractionFailed: boolean;
    live: boolean;
    navigation: Command[];
}) {
    const [entry, setEntry] = useState(initialEntry);
    const save = useEntrySave(initialEntry);
    return (
        <article aria-labelledby="entry-title">
            <ReaderHeader entry={entry} />
            <ReaderMedia entry={entry} onChange={setEntry} />
            <ReaderBody entry={entry} />
            {canExtractArticle(entry) ? <ArticleExtraction entry={entry} onChange={setEntry} initiallyExtracted={initiallyExtracted} extractionFailed={extractionFailed} /> : null}
            {save ? <SaveFiling save={save} /> : null}
            <ReaderRail entry={entry} save={save} />
            {live ? <ReaderCommands entry={entry} navigation={navigation} onChange={setEntry} save={save} /> : null}
        </article>
    );
}

const READER = 'Reader';

// The save's commands register first, as a child, so they lead the menu.
function ReaderCommands({ entry, save, navigation, onChange }: {
    entry: ReaderEntry;
    save: EntrySave | null;
    navigation: Command[];
    onChange: (entry: ReaderEntry) => void;
}) {
    const [, navigate] = useLocation();
    const sourceLink = readerSourceHref(entry.source);
    useCommands([
        ...originalCommands(entry),
        ...(sourceLink ? [{ id: 'entry:source', group: READER, label: `Go to ${sourceTitle(entry)}`, run: () => navigate(sourceLink) }] : []),
        ...imageCommands(entry, onChange),
        ...navigation,
    ]);
    return save ? <SaveCommands save={save} /> : null;
}

function originalCommands(entry: ReaderEntry): Command[] {
    const original = httpURL(entry.url);
    if (!original) return [];
    return [
        { id: 'entry:original', group: READER, label: sourceActionLabel(entry), run: () => openInTab(original) },
        { id: 'entry:copy-link', group: READER, label: 'Copy link to original', run: () => copied(original, 'Link copied') },
    ];
}

function imageCommands(entry: ReaderEntry, onChange: (entry: ReaderEntry) => void): Command[] {
    const { newsletter } = entry;
    if (!newsletter?.hasBlockedRemoteImages || newsletter.remoteImagesAllowed) return [];
    return [{
        id: 'entry:images',
        group: READER,
        label: 'Load remote images',
        detail: 'This message only',
        run: async () => onChange(await allowRemoteImages(entry, newsletter)),
    }];
}

function SaveCommands({ save }: { save: EntrySave }) {
    const { saved, tags } = useSaveState(save);
    useCommands([
        {
            id: 'entry:save',
            group: READER,
            label: 'Saved to Library',
            keywords: ['save', 'bookmark', 'library', 'remove'],
            toggle: {
                on: saved,
                set: (on) => {
                    if (!on) return applied(announceRemove(save, tags));
                    const saving = save.save();
                    announceSave(save);
                    return applied(saving);
                },
            },
        },
        { id: 'entry:tags', group: READER, label: 'Edit tags', detail: tags.join(', '), step: { kind: 'tags', title: 'Tags', tags, suggestionsPath: '/api/saves/tags', save: (next) => applied(save.setTags(next)) } },
    ]);
    return null;
}

// The save toasts its own failure, so the menu is only told to roll back.
async function applied(change: Promise<boolean>) {
    if (!(await change)) throw new CommandProblem('');
}

function ReaderMedia({ entry, onChange }: { entry: ReaderEntry; onChange: (entry: ReaderEntry) => void }) {
    if (entry.newsletter) return <NewsletterImages entry={entry} newsletter={entry.newsletter} onChange={onChange} />;
    if (entry.contentType === 'video') return <ReaderVideo youtube={youtubeID(entry.url)} video={readerMetadata(entry.metadata).video} title={entry.title || 'Video'} />;
    return null;
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

function ReaderBody({ entry }: { entry: ReaderEntry }) {
    const [font] = useReaderFont();
    if (entry.body?.trim()) return <div className="reader-body" data-font={font} dangerouslySetInnerHTML={{ __html: readerBody(entry) }} />;
    if (entry.contentType === 'video') return null;
    return <p className="text-muted-foreground">{httpURL(entry.url) ? 'There is no readable content here. Open the original to continue.' : 'No content is available for this entry.'}</p>;
}

function sourceTitle(entry: ReaderEntry): string {
    const url = httpURL(entry.source.siteUrl) || httpURL(entry.source.feedUrl);
    return entry.source.title || (url ? new URL(url).hostname : 'Unknown source');
}

function readerTitle(state: EntryState): string {
    return state.status === 'loaded' ? entryTitle(state.entry) : 'Reader';
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
            <p className="mb-4 text-sm text-muted-foreground"><SourceLink entry={entry}>{source}</SourceLink>{labels[entry.contentType]}</p>
            <h1 className="text-3xl font-medium text-balance break-words" id="entry-title">{entryTitle(entry)}</h1>
            <div className="mt-4 flex flex-wrap gap-x-3 gap-y-1 text-sm text-subtle-foreground">
                {author ? <span>{author}</span> : null}
                {!Number.isNaN(date.getTime()) ? <time dateTime={entry.publishedAt}>{date.toLocaleDateString(undefined, { day: 'numeric', month: 'long', year: 'numeric' })}</time> : null}
            </div>
        </header>
    );
}

function SourceLink({ entry, children }: { entry: ReaderEntry; children: string }) {
    const href = readerSourceHref(entry.source);
    if (!href) return children;
    return <Link className="rounded-sm hover:text-foreground" href={href}>{children}</Link>;
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
