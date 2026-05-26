import { Cancel01Icon, PlayIcon } from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import DOMPurify from 'dompurify';
import { useEffect, useMemo, useState } from 'react';

import { Favicon } from '@/components/favicon';
import { ReaderRail } from '@/components/reader-rail';
import type {
    ExtractedToggleState,
    SavedToggle,
} from '@/components/reader-rail';
import { buttonVariants } from '@/components/ui/button-variants';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { digestHref, PATHS, sourceHref } from '@/lib/paths';
import { cn, safeHref } from '@/lib/utils';

type ContentType = 'blogpost' | 'microblog' | 'video' | 'podcast';

type Source = {
    feedUrl: string;
    title: string | null;
    siteUrl: string | null;
    faviconUrl: string | null;
};

type SavedState = { rkey: string };

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
};

type State =
    | { kind: 'loading' }
    | { kind: 'ok'; entry: Entry }
    | { kind: 'error' };

type Override = 'auto' | 'feed' | 'extracted';

function slugFromLocation(): string | null {
    const path = window.location.pathname;
    const prefix = `${PATHS.entry}/`;
    if (!path.startsWith(prefix)) return null;
    const slug = path.slice(prefix.length);
    return slug.length > 0 ? slug : null;
}

function backHrefFromLocation(): string {
    const params = new URLSearchParams(window.location.search);
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
    const slug = slugFromLocation();
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
                const r = await fetch(`/api/entries/${slug}`, {
                    credentials: 'same-origin',
                });
                if (!r.ok) throw new Error(String(r.status));
                const entry = (await r.json()) as Entry;
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
            <Shell>
                <p className="text-muted-foreground">Loading…</p>
            </Shell>
        );
    }

    if (state.kind === 'error') {
        return (
            <Shell>
                <p className="text-muted-foreground">
                    Couldn't open this entry.
                </p>
            </Shell>
        );
    }

    if (state.entry.contentType === 'video') {
        return <WatchView entry={state.entry} />;
    }

    return <ReaderView entry={state.entry} />;
}

function Shell({ children }: { children: React.ReactNode }) {
    return (
        <div className="min-h-svh bg-card">
            <Header />
            <article className="mx-auto w-full max-w-2xl px-4 pt-8 pb-24 sm:px-6">
                {children}
            </article>
        </div>
    );
}

function Header() {
    const back = backHrefFromLocation();
    return (
        <header className="sticky top-0 z-10 flex h-14 items-center px-4 sm:px-6">
            <a
                href={back}
                aria-label="Back"
                className="inline-flex size-9 items-center justify-center rounded-xl text-muted-foreground transition-colors duration-200 ease-out outline-none hover:text-foreground focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring focus-visible:outline-solid"
            >
                <HugeiconsIcon icon={Cancel01Icon} className="size-5" />
            </a>
        </header>
    );
}

function ReaderView({ entry }: { entry: Entry }) {
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
        fetch(`/api/entries/${entry.entrySlug}/extract`, {
            method: 'POST',
            credentials: 'same-origin',
        })
            .then(async (r) => {
                if (!r.ok) throw new Error(String(r.status));
                const next = (await r.json()) as Entry;
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
    const savedToggle: SavedToggle = {
        initial: entry.savedState,
        itemUrl: entry.url,
        feedUrl: entry.source.feedUrl ?? null,
    };

    return (
        <div className="min-h-svh bg-card">
            <Header />
            <ReaderRail
                sourceUrl={sourceLink ?? null}
                extractedToggle={{ state: toggleState, onClick: onToggleClick }}
                savedToggle={savedToggle}
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

function WatchView({ entry }: { entry: Entry }) {
    const embed = useMemo(() => resolveVideoEmbed(entry.url), [entry.url]);
    const sourceLink = safeHref(entry.url);
    const savedToggle: SavedToggle = {
        initial: entry.savedState,
        itemUrl: entry.url,
        feedUrl: entry.source.feedUrl ?? null,
    };

    return (
        <div className="min-h-svh bg-card">
            <Header />
            <ReaderRail
                sourceUrl={sourceLink ?? null}
                savedToggle={savedToggle}
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
                    <HugeiconsIcon
                        icon={PlayIcon}
                        className="size-7 translate-x-[1px]"
                    />
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
    return (
        <div className="flex items-center gap-2 font-sans">
            <Favicon src={source.faviconUrl} />
            <span className="line-clamp-1 text-sm font-light text-muted-foreground">
                {label}
            </span>
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

// Body is server-sanitized (bluemonday UGC) at ingest. DOMPurify runs as
// client-side defense-in-depth so a server-side regression can't escape.
function ReaderBody({ html }: { html: string }) {
    const clean = useMemo(() => DOMPurify.sanitize(html), [html]);
    return (
        <div
            className="font-serif text-base leading-relaxed text-foreground [&_a]:text-primary [&_a]:underline-offset-4 [&_a:hover]:underline [&_blockquote]:border-l-2 [&_blockquote]:border-border [&_blockquote]:pl-4 [&_blockquote]:italic [&_h2]:mt-8 [&_h2]:mb-3 [&_h2]:font-sans [&_h3]:mt-6 [&_h3]:mb-2 [&_h3]:font-sans [&_img]:rounded-2xl [&_ol]:mb-5 [&_ol]:list-decimal [&_ol]:pl-6 [&_p]:mb-5 [&_pre]:overflow-x-auto [&_pre]:rounded-xl [&_pre]:bg-muted [&_pre]:p-4 [&_pre]:font-mono [&_pre]:text-sm [&_ul]:mb-5 [&_ul]:list-disc [&_ul]:pl-6"
            dangerouslySetInnerHTML={{ __html: clean }}
        />
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

function readAuthor(metadata: string | null | undefined): string | null {
    if (!metadata) return null;
    try {
        const parsed = JSON.parse(metadata) as { author?: unknown };
        return typeof parsed.author === 'string' ? parsed.author : null;
    } catch {
        return null;
    }
}

function hostnameOf(url: string): string | null {
    try {
        const u = new URL(url);
        return u.hostname.replace(/^www\./, '');
    } catch {
        return null;
    }
}

function formatDate(iso: string): string {
    try {
        const d = new Date(iso);
        return d.toLocaleDateString(undefined, {
            year: 'numeric',
            month: 'short',
            day: 'numeric',
        });
    } catch {
        return iso;
    }
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
