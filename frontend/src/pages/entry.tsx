import { PlayIcon } from '@proicons/react';
import DOMPurify from 'dompurify';
import { useEffect, useMemo, useState } from 'react';
import { Link, useParams, useSearch } from 'wouter';

import { Favicon } from '@/components/favicon';
import { ReaderBody } from '@/components/reader-body';
import { ReaderRail } from '@/components/reader-rail';
import type { ExtractedToggleState } from '@/components/reader-rail';
import { ReaderHeader, ReaderShell } from '@/components/reader-shell';
import { ShareComposer } from '@/components/share-composer';
import { buttonVariants } from '@/components/ui/button-variants';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { useGoBackOr } from '@/hooks/use-go-back-or';
import { useKeyboard } from '@/hooks/use-keyboard';
import { useSaveToggle } from '@/hooks/use-save-toggle';
import type { SavedToggle } from '@/hooks/use-save-toggle';
import { useShareToggle } from '@/hooks/use-share-toggle';
import type { ShareToggle } from '@/hooks/use-share-toggle';
import { api } from '@/lib/api';
import { formatDate } from '@/lib/date';
import { readAuthor } from '@/lib/entry-meta';
import { digestHref, sourceHref } from '@/lib/paths';
import { cn, hostnameOf, safeHref } from '@/lib/utils';

type ContentType = 'blogpost' | 'microblog' | 'video';

type Source = {
    feedUrl: string;
    title: string | null;
    siteUrl: string | null;
    faviconUrl: string | null;
    rkey?: string;
};

type SavedState = { rkey: string };
type SharedState = { rkey: string };

type Entry = {
    id: number;
    entrySlug: string;
    title: string | null;
    url: string;
    contentType: ContentType;
    publishedAt: string;
    source: Source;
    body: string | null;
    metadata?: string | null;
    savedState: SavedState | null;
    sharedState: SharedState | null;
};

type State =
    | { kind: 'loading' }
    | { kind: 'ok'; entry: Entry }
    | { kind: 'error' };

type Override = 'auto' | 'feed' | 'extracted';

function backHrefFromLocation(search: string): string {
    const params = new URLSearchParams(search);
    const rkey = params.get('fromSource');
    if (rkey && /^[A-Za-z0-9._~-]+$/.test(rkey)) {
        return sourceHref(rkey);
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
    // A path-less Standardfeed doc has no canonical URL: no save index key (itemUrl), no share comment target.
    const canSave = entry.url !== '';
    const savedToggle: SavedToggle = {
        initial: entry.savedState,
        itemUrl: entry.url,
        feedUrl: entry.source.feedUrl ?? null,
    };
    const save = useSaveToggle(savedToggle);
    const shareToggle: ShareToggle = {
        initial: entry.sharedState,
        entrySlug: entry.entrySlug,
        canComment: canSave,
    };
    const share = useShareToggle(shareToggle);

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
                share={share}
            />
            <ShareComposer share={share} />
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

function WatchView({ entry, backHref }: { entry: Entry; backHref: string }) {
    const goBackOr = useGoBackOr();
    const embed = useMemo(() => resolveVideoEmbed(entry.url), [entry.url]);
    const sourceLink = safeHref(entry.url);
    const savedToggle: SavedToggle = {
        initial: entry.savedState,
        itemUrl: entry.url,
        feedUrl: entry.source.feedUrl ?? null,
    };
    const save = useSaveToggle(savedToggle);
    const shareToggle: ShareToggle = {
        initial: entry.sharedState,
        entrySlug: entry.entrySlug,
        canComment: entry.url !== '',
    };
    const share = useShareToggle(shareToggle);

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
                share={share}
                showProgress={false}
            />
            <ShareComposer share={share} />
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
    const label = source.title ?? source.feedUrl;
    const content = (
        <>
            <Favicon src={source.faviconUrl} />
            <span className="line-clamp-1 text-sm font-light">{label}</span>
        </>
    );
    if (source.rkey) {
        return (
            <Link
                href={sourceHref(source.rkey)}
                className="flex w-fit items-center gap-2 rounded-sm font-sans text-muted-foreground transition-colors duration-200 ease-out outline-none hover:text-foreground focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring focus-visible:outline-solid"
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

function Byline({ entry }: { entry: Entry }) {
    const bits: string[] = [];
    const author = readAuthor(entry.metadata);
    if (author) bits.push(author);
    if (entry.publishedAt) bits.push(formatDate(entry.publishedAt));
    const host = hostnameOf(entry.url);
    if (host) bits.push(host);
    if (bits.length === 0) return null;
    return (
        <p className="font-sans text-sm font-light text-muted-foreground">
            {bits.join(' · ')}
        </p>
    );
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
