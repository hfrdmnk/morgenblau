import { useEffect, useState } from 'react';
import { useLocation, useParams } from 'wouter';
import { useSearch } from 'wouter/use-browser-location';

import { BackLink } from '@/components/back-link';
import { NewsletterImages } from '@/components/newsletter-images';
import { ReaderNotice } from '@/components/reader-notice';
import { ReaderRail } from '@/components/reader-rail';
import { ReaderYouTube } from '@/components/reader-youtube';
import { SaveFiling } from '@/components/save-filing';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import { Link } from '@/components/ui/link';
import { ReaderBody as ArticleBody } from '@/components/ui/reader-body';
import { VideoPlayer } from '@/components/ui/video';
import { Frame } from '@/design/frame';
import { Text } from '@/design/text';
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
        <Frame align="center" background="background" minHeight="screen">
            <Frame align="start" aria-label="Reader" as="nav" maxWidth="3xl" paddingTop={{ base: '2xl', sm: '4xl' }} paddingX={{ base: 'xl', sm: '3xl' }} width="full">
                <BackLink href={back} keyShortcut="Escape" label={backLabel} />
            </Frame>
            <Frame as="main" id="reader" maxWidth="3xl" paddingBottom="8xl" paddingTop={{ base: '5xl', sm: '6xl' }} paddingX={{ base: 'xl', sm: '3xl' }} width="full">
                <EntryLoader key={slug} navigation={navigation} slug={slug} />
            </Frame>
        </Frame>
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
        <Skeleton data={state.status === 'loaded' ? state : null} error={error} label="Loading entry" sample={SAMPLE_ENTRY}>
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
        <Frame align="start" gap="l" role="alert">
            <Text as="h1" size="3xl" tracking="tight" weight="medium">{title}</Text>
            <Text tone="muted">{description}</Text>
            {status === 401 ? <Text><Link href={PATHS.login} variant="underline">Sign in</Link></Text> : status !== 404 ? <Button variant="secondary" onClick={retry}>Try again</Button> : null}
        </Frame>
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
        <Frame aria-labelledby="entry-title" as="article">
            <ReaderHeader entry={entry} />
            <ReaderMedia entry={entry} onChange={setEntry} />
            <ReaderBody entry={entry} />
            {canExtractArticle(entry) ? <ArticleExtraction entry={entry} onChange={setEntry} initiallyExtracted={initiallyExtracted} extractionFailed={extractionFailed} /> : null}
            {save ? <SaveFiling save={save} /> : null}
            <ReaderRail entry={entry} save={save} />
            {live ? <ReaderCommands entry={entry} navigation={navigation} onChange={setEntry} save={save} /> : null}
        </Frame>
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
    return entry.contentType === 'video' ? <EntryVideo entry={entry} /> : null;
}

function EntryVideo({ entry }: { entry: ReaderEntry }) {
    const youtube = youtubeID(entry.url);
    const title = entry.title || 'Video';
    if (youtube) return <ReaderYouTube entrySlug={entry.entrySlug} title={title} videoID={youtube} />;
    return <ReaderVideo video={readerMetadata(entry.metadata).video} title={title} />;
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
        <Frame gap="xl">
            {extracted ? null : <ExtractPrompt onExtract={() => void extract()} pending={pending} />}
            <ReaderNotice notice={notice} />
        </Frame>
    );
}

function ExtractPrompt({ pending, onExtract }: { pending: boolean; onExtract: () => void }) {
    return (
        <Frame align="start" gap="m" paddingTop="5xl">
            <Text size="sm" tone="muted">Only seeing part of the article?</Text>
            <Button disabled={pending} onClick={onExtract} variant="secondary">{pending ? 'Loading full article…' : 'Load full article'}</Button>
        </Frame>
    );
}

function ReaderBody({ entry }: { entry: ReaderEntry }) {
    const [font] = useReaderFont();
    if (entry.body?.trim()) return <ArticleBody data-font={font} dangerouslySetInnerHTML={{ __html: readerBody(entry) }} />;
    if (entry.contentType === 'video') return null;
    return <Text tone="muted">{httpURL(entry.url) ? 'There is no readable content here. Open the original to continue.' : 'No content is available for this entry.'}</Text>;
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
        <Frame as="header" gap="l" paddingBottom="4xl">
            <Text size="sm" tone="muted"><SourceLink entry={entry}>{source}</SourceLink>{labels[entry.contentType]}</Text>
            <Text as="h1" break="words" id="entry-title" size="3xl" tracking="tight" weight="medium" wrap="balance">{entryTitle(entry)}</Text>
            <Frame direction="row" gapX="m" gapY="2xs" wrap>
                {author ? <Text as="span" size="sm" tone="subtle">{author}</Text> : null}
                {!Number.isNaN(date.getTime()) ? <Text as="time" dateTime={entry.publishedAt} size="sm" tone="subtle">{date.toLocaleDateString(undefined, { day: 'numeric', month: 'long', year: 'numeric' })}</Text> : null}
            </Frame>
        </Frame>
    );
}

function SourceLink({ entry, children }: { entry: ReaderEntry; children: string }) {
    const href = readerSourceHref(entry.source);
    if (!href) return children;
    return <Link href={href} variant="quiet">{children}</Link>;
}

function ReaderVideo({ video, title }: {
    video?: { url: string; type: string };
    title: string;
}) {
    if (video) {
        return (
            <Frame paddingBottom="4xl">
                <VideoPlayer aria-label={title} controls preload="none"><source src={video.url} type={video.type} /><Text>Your browser cannot play this video. Open it at the source.</Text></VideoPlayer>
            </Frame>
        );
    }
    return <Frame paddingBottom="4xl"><Text tone="muted">This video is available at its original source.</Text></Frame>;
}
