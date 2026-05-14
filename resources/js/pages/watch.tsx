import {
    Cancel01Icon,
    Globe02Icon,
    PlayIcon,
} from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import { Head, Link } from '@inertiajs/react';
import { useState } from 'react';

import { ReaderRail } from '@/components/reader-rail';
import { consume } from '@/routes';

type Entry = App.Data.Reader.WatchReaderData;

type WatchPageProps = {
    entry: Entry;
};

export default function WatchPage({ entry }: WatchPageProps) {
    return (
        <>
            <Head title={entry.title ?? 'Watch'} />
            <div className="min-h-svh bg-card">
                <header className="sticky top-0 z-10 flex h-14 items-center px-4 sm:px-6">
                    <Link
                        href={consume().url}
                        viewTransition
                        aria-label="Back to digest"
                        className="inline-flex size-9 items-center justify-center rounded-xl text-muted-foreground transition-colors duration-200 ease-out outline-none hover:text-foreground focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring focus-visible:outline-solid"
                    >
                        <HugeiconsIcon icon={Cancel01Icon} className="size-5" />
                    </Link>
                </header>

                <ReaderRail sourceUrl={entry.source_url} showProgress={false} />

                <article className="mx-auto w-full px-4 pt-8 pb-24 sm:px-6">
                    <header className="mx-auto mb-8 flex max-w-2xl flex-col gap-4">
                        <FeedLine entry={entry} />
                        {entry.title ? (
                            <h1 className="text-balance">{entry.title}</h1>
                        ) : null}
                        <Byline entry={entry} />
                    </header>

                    <div className="mx-auto mb-8 max-w-4xl">
                        <VideoPlayer entry={entry} />
                    </div>

                    {entry.description ? (
                        <div className="mx-auto max-w-2xl">
                            <Description html={entry.description} />
                        </div>
                    ) : null}
                </article>
            </div>
        </>
    );
}

function VideoPlayer({ entry }: { entry: Entry }) {
    const [playing, setPlaying] = useState(false);

    const surface =
        'aspect-video w-full overflow-hidden rounded-2xl bg-gray-100 dark:bg-gray-900';

    if (playing && entry.embed_url) {
        return (
            <div className={surface}>
                <iframe
                    src={entry.embed_url}
                    title={entry.title ?? 'Video player'}
                    allow="accelerometer; autoplay; encrypted-media; gyroscope; picture-in-picture; web-share"
                    allowFullScreen
                    className="h-full w-full border-0"
                />
            </div>
        );
    }

    if (!entry.embed_url) {
        if (!entry.thumbnail_url || !entry.source_url) {
            return null;
        }

        return (
            <a
                href={entry.source_url}
                target="_blank"
                rel="noopener noreferrer"
                className={`group relative block ${surface} outline-none focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring focus-visible:outline-solid`}
            >
                <Thumbnail src={entry.thumbnail_url} />
            </a>
        );
    }

    return (
        <button
            type="button"
            onClick={() => setPlaying(true)}
            aria-label="Play video"
            className={`group relative block ${surface} outline-none focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring focus-visible:outline-solid`}
        >
            {entry.thumbnail_url ? (
                <Thumbnail src={entry.thumbnail_url} />
            ) : null}
            <span
                aria-hidden
                className="absolute inset-0 flex items-center justify-center"
            >
                <span className="inline-flex size-16 items-center justify-center rounded-full bg-black/55 text-white transition-colors duration-200 ease-out group-hover:bg-black/70">
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

// Description is sanitized at ingest by HtmlSanitizer (Purify); YouTube ships
// plain text with literal newlines, so whitespace-pre-wrap renders the breaks.
function Description({ html }: { html: string }) {
    return (
        <div
            className="text-base whitespace-pre-wrap text-foreground [&_a]:text-primary [&_a]:underline-offset-4 [&_a:hover]:underline"
            dangerouslySetInnerHTML={{ __html: html }}
        />
    );
}

function FeedLine({ entry }: { entry: Entry }) {
    return (
        <div className="flex items-center gap-2">
            <Favicon src={entry.feed.favicon_url} />
            <p className="line-clamp-1 text-sm font-light text-muted-foreground">
                {entry.feed.display_title}
            </p>
        </div>
    );
}

function Byline({ entry }: { entry: Entry }) {
    const bits: string[] = [];

    if (entry.author) {
        bits.push(entry.author);
    }

    if (entry.published_at) {
        bits.push(formatDate(entry.published_at));
    }

    if (entry.source_domain) {
        bits.push(entry.source_domain);
    }

    if (bits.length === 0) {
        return null;
    }

    return (
        <p className="text-sm font-light text-muted-foreground">
            {bits.join(' · ')}
        </p>
    );
}

function Favicon({ src }: { src: string | null }) {
    const [errored, setErrored] = useState(false);

    if (!src || errored) {
        return (
            <HugeiconsIcon
                icon={Globe02Icon}
                className="size-4 text-muted-foreground"
            />
        );
    }

    return (
        <img
            src={src}
            alt=""
            className="size-4 rounded-sm"
            onError={() => setErrored(true)}
            loading="lazy"
        />
    );
}

function formatDate(iso: string): string {
    const then = new Date(iso);

    return then.toLocaleDateString(undefined, {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
    });
}
