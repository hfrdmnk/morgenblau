import { MailIcon, PhotoIcon, PlayIcon, SpinnerIcon } from '@proicons/react';
import DOMPurify from 'dompurify';
import { useEffect, useMemo, useState } from 'react';
import { Link, useParams, useSearch } from 'wouter';

import { Favicon } from '@/components/favicon';
import { MoveMessageDialog } from '@/components/newsletters/move-message-dialog';
import { ReaderBody } from '@/components/reader-body';
import { ReaderRail } from '@/components/reader-rail';
import type { ExtractedToggleState } from '@/components/reader-rail';
import { ReaderHeader, ReaderShell } from '@/components/reader-shell';
import { Button } from '@/components/ui/button';
import { buttonVariants } from '@/components/ui/button-variants';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { useGoBackOr } from '@/hooks/use-go-back-or';
import { useKeyboard } from '@/hooks/use-keyboard';
import { useSaveToggle } from '@/hooks/use-save-toggle';
import type { SavedToggle } from '@/hooks/use-save-toggle';
import { api } from '@/lib/api';
import { formatDate } from '@/lib/date';
import { readAuthor } from '@/lib/entry-meta';
import type {
    NewsletterMessageMeta,
    NewsletterSource,
} from '@/lib/newsletters';
import { toastMutationError } from '@/lib/mutation-toast';
import {
    digestHref,
    newsletterSourceHref,
    sourceHref,
} from '@/lib/paths';
import { cn, hostnameOf, safeHref } from '@/lib/utils';

type ContentType = 'blogpost' | 'microblog' | 'newsletter' | 'video';

type Source = {
    kind?: 'rss' | 'standardfeed' | 'newsletter';
    id?: string;
    feedUrl?: string;
    title: string | null;
    siteUrl: string | null;
    faviconUrl: string | null;
    rkey?: string;
};

type SavedState =
    | { rkey: string; kind?: 'feed' }
    | { kind: 'newsletter'; id: string };

type Entry = {
    id: number | string;
    entrySlug: string;
    title: string | null;
    url?: string | null;
    contentType: ContentType;
    publishedAt: string;
    source: Source;
    body: string | null;
    metadata?: string | null;
    savedState: SavedState | null;
    newsletter?: NewsletterMessageMeta;
};

type State =
    | { kind: 'loading' }
    | { kind: 'ok'; entry: Entry }
    | { kind: 'error' };

type Override = 'auto' | 'feed' | 'extracted';

type NewsletterImageState = {
    body: string | null;
    remoteImagesAllowed: boolean;
    hasBlockedRemoteImages: boolean;
};

function isNewsletterEntry(
    entry: Entry,
): entry is Entry & { newsletter: NewsletterMessageMeta } {
    return entry.contentType === 'newsletter' && entry.newsletter !== undefined;
}

function feedSavedToggle(entry: Entry): SavedToggle {
    return {
        kind: 'feed',
        initial: feedSavedInitial(entry.savedState),
        itemUrl: entry.url || '',
        feedUrl: entry.source.feedUrl || null,
    };
}

function feedSavedInitial(savedState: SavedState | null) {
    if (!savedState || !('rkey' in savedState)) return null;
    return { rkey: savedState.rkey };
}

function useFeedSave(entry: Entry) {
    return useSaveToggle(feedSavedToggle(entry));
}

function backHrefFromLocation(search: string): string {
    const params = new URLSearchParams(search);
    const rkey = params.get('fromSource');
    if (rkey && /^[A-Za-z0-9._~-]+$/.test(rkey)) {
        return sourceHref(rkey);
    }
    const newsletterId = params.get('fromNewsletter');
    if (newsletterId) {
        return newsletterSourceHref(newsletterId);
    }
    const date = params.get('from');
    if (date && /^\d{4}-\d{2}-\d{2}$/.test(date)) {
        return digestHref(date);
    }
    return digestHref();
}

export function Entry() {
    const { slug } = useParams<{ slug: string }>();
    const search = useSearch();
    const backHref = backHrefFromLocation(search);
    const [state, setState] = useState<State>(
        slug ? { kind: 'loading' } : { kind: 'error' },
    );

    useDocumentTitle(
        state.kind === 'ok' ? state.entry.title ?? 'Reader' : 'Reader',
    );

    useEffect(() => {
        if (!slug) return;
        let cancelled = false;
        const load = async () => {
            try {
                const entry = await api<Entry>(`/api/entries/${slug}`);
                if (!cancelled) setState({ kind: 'ok', entry });
            } catch {
                if (!cancelled) setState({ kind: 'error' });
            }
        };
        load();
        return () => {
            cancelled = true;
        };
    }, [slug]);

    if (state.kind === 'loading') {
        return (
            <ReaderShell backHref={backHref}>
                <p className="text-muted-foreground">Loading…</p>
            </ReaderShell>
        );
    }

    if (state.kind === 'error') {
        return (
            <ReaderShell backHref={backHref}>
                <p className="text-muted-foreground">
                    Couldn't open this entry.
                </p>
            </ReaderShell>
        );
    }

    if (state.entry.contentType === 'video') {
        return <WatchView entry={state.entry} backHref={backHref} />;
    }
    if (isNewsletterEntry(state.entry)) {
        return (
            <NewsletterReaderView
                key={state.entry.entrySlug}
                entry={state.entry}
                backHref={backHref}
            />
        );
    }

    return <ReaderView entry={state.entry} backHref={backHref} />;
}

function ReaderView({ entry, backHref }: { entry: Entry; backHref: string }) {
    const goBackOr = useGoBackOr();
    const [override, setOverride] = useState<Override>('auto');
    const [loading, setLoading] = useState(false);
    const [manualFailed, setManualFailed] = useState(false);
    const [extracted, setExtracted] = useState<string | null>(null);

    const hasExtracted = extracted !== null && extracted !== '';

    const toggleState: ExtractedToggleState = loading
        ? 'loading'
        : override === 'extracted'
          ? 'active'
          : 'inactive';

    const onToggleClick = () => {
        if (loading) return;

        if (toggleState === 'active') {
            setOverride('feed');
            setManualFailed(false);
            return;
        }

        if (hasExtracted) {
            setOverride('extracted');
            setManualFailed(false);
            return;
        }

        setLoading(true);
        api<Entry>(`/api/entries/${entry.entrySlug}/extract`, { method: 'POST' })
            .then((next) => {
                const nextBody = next.body ?? '';
                if (nextBody !== '') {
                    setExtracted(nextBody);
                    setOverride('extracted');
                    setManualFailed(false);
                } else {
                    setManualFailed(true);
                }
            })
            .catch(() => setManualFailed(true))
            .finally(() => setLoading(false));
    };

    const body = manualFailed
        ? null
        : override === 'extracted'
          ? extracted ?? entry.body
          : entry.body;

    const sourceLink = safeHref(entry.url);
    // A path-less Standardfeed document has no canonical URL to use as a save index key.
    const canSave = Boolean(entry.url);
    const save = useFeedSave(entry);

    useKeyboard({
        Escape: () => {
            goBackOr(backHref);
        },
        b: () => {
            if (canSave) save.onToggle();
        },
        o: () => {
            if (sourceLink) {
                window.open(sourceLink, '_blank', 'noopener,noreferrer');
            }
        },
        m: () => onToggleClick(),
    });

    return (
        <div className="min-h-svh bg-card">
            <ReaderHeader backHref={backHref} />
            <ReaderRail
                sourceUrl={sourceLink ?? null}
                extractedToggle={{ state: toggleState, onClick: onToggleClick }}
                save={canSave ? save : undefined}
            />
            <article className="mx-auto w-full max-w-2xl px-4 pt-8 pb-24 sm:px-6">
                <header className="mb-8 flex flex-col gap-4">
                    <FeedLine source={entry.source} />
                    {entry.title ? (
                        <h1 className="text-2xl font-medium tracking-tight text-balance text-foreground">
                            {entry.title}
                        </h1>
                    ) : null}
                    <Byline entry={entry} />
                </header>

                {manualFailed && sourceLink ? (
                    <ManualFailureFallback sourceUrl={sourceLink} />
                ) : body ? (
                    <ReaderBody html={body} />
                ) : null}
            </article>
        </div>
    );
}

function newsletterSavedToggle(
    entry: Entry & { newsletter: NewsletterMessageMeta },
): SavedToggle {
    const initial =
        entry.savedState?.kind === 'newsletter' ? entry.savedState : null;
    return {
        kind: 'newsletter',
        initial,
        messageId: entry.newsletter.messageId,
    };
}

function newsletterReaderSource(
    original: Source,
    moved: NewsletterSource | null,
): Source {
    if (!moved) return original;
    return {
        kind: 'newsletter',
        id: moved.id,
        title: moved.title,
        siteUrl: null,
        faviconUrl: null,
    };
}

function initialNewsletterImages(
    entry: Entry & { newsletter: NewsletterMessageMeta },
): NewsletterImageState {
    return {
        body: entry.body,
        remoteImagesAllowed: entry.newsletter.remoteImagesAllowed,
        hasBlockedRemoteImages: entry.newsletter.hasBlockedRemoteImages,
    };
}

function useNewsletterImages(
    entry: Entry & { newsletter: NewsletterMessageMeta },
) {
    const [loaded, setLoaded] = useState<NewsletterImageState | null>(null);
    const [loading, setLoading] = useState(false);
    const images = loaded ?? initialNewsletterImages(entry);

    const load = async () => {
        if (loading) return;
        setLoading(true);
        try {
            const next = await api<NewsletterImageState>(
                `/api/newsletters/messages/${encodeURIComponent(entry.newsletter.messageId)}/images`,
                { method: 'POST' },
            );
            setLoaded(next);
        } catch (error) {
            toastMutationError(error, "Couldn't load these images. Try again.");
        } finally {
            setLoading(false);
        }
    };

    return { images, loading, load };
}

function NewsletterMessageContent({
    entry,
}: {
    entry: Entry & { newsletter: NewsletterMessageMeta };
}) {
    const { images, loading, load } = useNewsletterImages(entry);
    const showImageNotice =
        images.hasBlockedRemoteImages && !images.remoteImagesAllowed;
    return (
        <>
            <RemoteImageNotice
                visible={showImageNotice}
                loading={loading}
                onLoad={load}
            />
            {images.body ? <ReaderBody html={images.body} /> : null}
        </>
    );
}

function RemoteImageNotice({
    visible,
    loading,
    onLoad,
}: {
    visible: boolean;
    loading: boolean;
    onLoad: () => void;
}) {
    if (!visible) return null;
    return (
        <div className="mb-6 flex items-center justify-between gap-4 rounded-xl bg-muted px-3 py-2.5 font-sans">
            <p className="text-label text-muted-foreground">
                Remote images are blocked to protect your privacy.
            </p>
            <Button
                type="button"
                variant="secondary"
                size="sm"
                disabled={loading}
                onClick={onLoad}
            >
                {loading ? (
                    <SpinnerIcon className="size-4 motion-safe:animate-spin" />
                ) : (
                    <PhotoIcon className="size-4" />
                )}
                Load images
            </Button>
        </div>
    );
}

function NewsletterReaderView({
    entry,
    backHref,
}: {
    entry: Entry & { newsletter: NewsletterMessageMeta };
    backHref: string;
}) {
    const goBackOr = useGoBackOr();
    const [moveOpen, setMoveOpen] = useState(false);
    const [movedSource, setMovedSource] = useState<NewsletterSource | null>(null);
    const source = newsletterReaderSource(entry.source, movedSource);
    const save = useSaveToggle(newsletterSavedToggle(entry));

    useKeyboard({
        Escape: () => goBackOr(backHref),
        b: () => save.onToggle(),
    });

    return (
        <div className="min-h-svh bg-card">
            <ReaderHeader backHref={backHref} />
            <ReaderRail
                sourceUrl={null}
                save={save}
                move={{ onClick: () => setMoveOpen(true) }}
            />
            <article className="mx-auto w-full max-w-2xl px-4 pt-8 pb-24 sm:px-6">
                <header className="mb-8 flex flex-col gap-4">
                    <FeedLine source={source} />
                    {entry.title ? (
                        <h1 className="text-display text-balance">
                            {entry.title}
                        </h1>
                    ) : null}
                    <NewsletterByline entry={entry} />
                </header>

                <NewsletterMessageContent entry={entry} />
            </article>
            <MoveMessageDialog
                open={moveOpen}
                onOpenChange={setMoveOpen}
                messageId={entry.newsletter.messageId}
                currentSourceId={source.id}
                onMoved={setMovedSource}
            />
        </div>
    );
}

function WatchView({ entry, backHref }: { entry: Entry; backHref: string }) {
    const goBackOr = useGoBackOr();
    const embed = useMemo(() => resolveVideoEmbed(entry.url ?? ''), [entry.url]);
    const sourceLink = safeHref(entry.url);
    const save = useFeedSave(entry);

    useKeyboard({
        Escape: () => {
            goBackOr(backHref);
        },
        b: () => save.onToggle(),
        o: () => {
            if (sourceLink) {
                window.open(sourceLink, '_blank', 'noopener,noreferrer');
            }
        },
    });

    return (
        <div className="min-h-svh bg-card">
            <ReaderHeader backHref={backHref} />
            <ReaderRail
                sourceUrl={sourceLink ?? null}
                save={save}
                showProgress={false}
            />
            <article className="mx-auto w-full px-4 pt-8 pb-24 sm:px-6">
                <header className="mx-auto mb-8 flex max-w-2xl flex-col gap-4">
                    <FeedLine source={entry.source} />
                    {entry.title ? (
                        <h1 className="text-2xl font-medium tracking-tight text-balance text-foreground">
                            {entry.title}
                        </h1>
                    ) : null}
                    <Byline entry={entry} />
                </header>

                <div className="mx-auto mb-8 max-w-4xl">
                    <VideoPlayer
                        embedUrl={embed?.embedUrl ?? null}
                        thumbnailUrl={embed?.thumbnailUrl ?? null}
                        sourceUrl={sourceLink ?? null}
                        title={entry.title}
                    />
                </div>

                {entry.body ? (
                    <div className="mx-auto max-w-2xl">
                        <Description html={entry.body} />
                    </div>
                ) : null}
            </article>
        </div>
    );
}

function VideoPlayer({
    embedUrl,
    thumbnailUrl,
    sourceUrl,
    title,
}: {
    embedUrl: string | null;
    thumbnailUrl: string | null;
    sourceUrl: string | null;
    title: string | null;
}) {
    const [playing, setPlaying] = useState(false);
    const surface =
        'aspect-video w-full overflow-hidden rounded-2xl bg-gray-100 dark:bg-gray-900';

    if (playing && embedUrl) {
        return (
            <div className={surface}>
                <iframe
                    src={embedUrl}
                    title={title ?? 'Video player'}
                    allow="accelerometer; autoplay; encrypted-media; gyroscope; picture-in-picture; web-share"
                    allowFullScreen
                    className="h-full w-full border-0"
                />
            </div>
        );
    }

    if (!embedUrl) {
        if (!thumbnailUrl || !sourceUrl) return null;
        return (
            <a
                href={sourceUrl}
                target="_blank"
                rel="noopener noreferrer"
                className={cn(
                    'group relative block outline-none focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring focus-visible:outline-solid',
                    surface,
                )}
            >
                <Thumbnail src={thumbnailUrl} />
            </a>
        );
    }

    return (
        <button
            type="button"
            onClick={() => setPlaying(true)}
            aria-label="Play video"
            className={cn(
                'group relative block outline-none focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring focus-visible:outline-solid',
                surface,
            )}
        >
            {thumbnailUrl ? <Thumbnail src={thumbnailUrl} /> : null}
            <span
                aria-hidden
                className="absolute inset-0 flex items-center justify-center"
            >
                <span className="inline-flex size-16 items-center justify-center rounded-full bg-black/30 text-white backdrop-blur-lg transition-colors duration-200 ease-out group-hover:bg-black/50">
                    <PlayIcon className="size-7 translate-x-[1px]" />
                </span>
            </span>
        </button>
    );
}

function Thumbnail({ src }: { src: string }) {
    return (
        <img
            src={src}
            alt=""
            loading="eager"
            className="absolute inset-0 h-full w-full object-cover"
        />
    );
}

function FeedLine({ source }: { source: Source }) {
    const label = source.title ?? source.feedUrl ?? 'Newsletter';
    const href = sourcePageHref(source);
    const content = (
        <>
            <SourceIcon source={source} />
            <span className="line-clamp-1 text-sm font-light">{label}</span>
        </>
    );
    if (href) {
        return (
            <Link
                href={href}
                className="flex w-fit items-center gap-2 rounded-sm font-sans text-muted-foreground outline-none transition-colors duration-200 ease-out hover:text-foreground focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring focus-visible:outline-solid"
            >
                {content}
            </Link>
        );
    }
    return (
        <div className="flex items-center gap-2 font-sans text-muted-foreground">
            {content}
        </div>
    );
}

function SourceIcon({ source }: { source: Source }) {
    return source.kind === 'newsletter' ? (
        <MailIcon className="size-4" />
    ) : (
        <Favicon src={source.faviconUrl} />
    );
}

function sourcePageHref(source: Source): string | null {
    if (source.rkey) return sourceHref(source.rkey);
    if (source.kind !== 'newsletter' || !source.id) return null;
    return newsletterSourceHref(source.id);
}

function Byline({ entry }: { entry: Entry }) {
    const bits: string[] = [];
    const author = readAuthor(entry.metadata);
    if (author) bits.push(author);
    if (entry.publishedAt) bits.push(formatDate(entry.publishedAt));
    const host = entry.url ? hostnameOf(entry.url) : null;
    if (host) bits.push(host);
    if (bits.length === 0) return null;
    return (
        <p className="font-sans text-sm font-light text-muted-foreground">
            {bits.join(' · ')}
        </p>
    );
}

function NewsletterByline({
    entry,
}: {
    entry: Entry & { newsletter: NewsletterMessageMeta };
}) {
    const bits = newsletterBylineBits(entry);
    return (
        <p className="font-sans text-label text-muted-foreground">
            {bits.join(' · ')}
        </p>
    );
}

function newsletterBylineBits(
    entry: Entry & { newsletter: NewsletterMessageMeta },
): string[] {
    const received = entry.publishedAt ? formatDate(entry.publishedAt) : null;
    return [
        newsletterSender(entry.newsletter),
        received,
        originalSentDate(entry.newsletter.sentAt, entry.publishedAt),
    ].filter((bit): bit is string => Boolean(bit));
}

function newsletterSender(newsletter: NewsletterMessageMeta): string {
    return newsletter.senderName || newsletter.senderAddress;
}

function originalSentDate(
    sentAt: string | null | undefined,
    receivedAt: string,
): string | null {
    if (!sentAt || sentAt === receivedAt) return null;
    return `Sent ${formatDate(sentAt)}`;
}

function Description({ html }: { html: string }) {
    const clean = useMemo(() => DOMPurify.sanitize(html), [html]);
    return (
        <div
            className="text-base whitespace-pre-wrap text-foreground [&_a]:text-primary [&_a]:underline-offset-4 [&_a:hover]:underline"
            dangerouslySetInnerHTML={{ __html: clean }}
        />
    );
}

function ManualFailureFallback({ sourceUrl }: { sourceUrl: string }) {
    return (
        <div className="flex justify-start">
            <a
                href={sourceUrl}
                target="_blank"
                rel="noopener noreferrer"
                className={buttonVariants({ variant: 'secondary' })}
            >
                Open on original site
            </a>
        </div>
    );
}

// Mirrors VideoEmbed.php: resolves a YouTube URL to embed + thumbnail.
function resolveVideoEmbed(
    link: string,
): { embedUrl: string; thumbnailUrl: string } | null {
    if (!link) return null;
    let parsed: URL;
    try {
        parsed = new URL(link);
    } catch {
        return null;
    }

    const host = parsed.hostname.toLowerCase();
    const youtubeHosts = new Set([
        'youtube.com',
        'www.youtube.com',
        'm.youtube.com',
        'youtu.be',
    ]);
    if (!youtubeHosts.has(host)) return null;

    let videoId: string | null = null;
    const path = parsed.pathname;

    if (host === 'youtu.be') {
        videoId = path.replace(/^\//, '');
    } else if (path === '/watch') {
        videoId = parsed.searchParams.get('v');
    } else {
        const m = /^\/(?:embed|v|shorts)\/([^/?#]+)/.exec(path);
        if (m) videoId = m[1];
    }

    if (!videoId || !/^[A-Za-z0-9_-]{11}$/.test(videoId)) return null;

    return {
        embedUrl: `https://www.youtube-nocookie.com/embed/${videoId}?autoplay=1&rel=0`,
        thumbnailUrl: `https://i.ytimg.com/vi/${videoId}/hqdefault.jpg`,
    };
}
