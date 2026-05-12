import { Cancel01Icon, Globe02Icon } from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import { Head, Link, router } from '@inertiajs/react';
import { useState } from 'react';

import { extract } from '@/actions/App/Http/Controllers/Reader/EntryController';
import { ReaderRail } from '@/components/reader-rail';
import type { ExtractedToggleState } from '@/components/reader-rail';
import { buttonVariants } from '@/components/ui/button';
import { consume } from '@/routes';

type Entry = App.Data.Reader.EntryReaderData;

type EntryPageProps = {
    entry: Entry;
};

// Session-only override the user picks via the rail toggle. Resets to 'auto'
// on every mount — refreshing the page returns to the auto-decide default.
type Override = 'auto' | 'feed' | 'extracted';

export default function EntryPage({ entry }: EntryPageProps) {
    const [override, setOverride] = useState<Override>('auto');
    const [loading, setLoading] = useState(false);
    const [manualFailed, setManualFailed] = useState(false);

    const hasExtracted =
        entry.extracted_body !== null && entry.extracted_body !== '';

    const toggleState: ExtractedToggleState = loading
        ? 'loading'
        : override === 'extracted'
          ? 'active'
          : override === 'auto' && entry.auto_choice === 'extracted'
            ? 'active'
            : 'inactive';

    const onToggleClick = () => {
        if (loading) {
            return;
        }

        // Currently rendering extracted — flip back to feed body, no network.
        if (toggleState === 'active') {
            setOverride('feed');
            setManualFailed(false);

            return;
        }

        // Cached extract on hand — flip to extracted, no network.
        if (hasExtracted) {
            setOverride('extracted');
            setManualFailed(false);

            return;
        }

        // No cached extract — go fetch synchronously.
        setLoading(true);
        router.post(
            extract({ slug: entry.entry_slug }).url,
            {},
            {
                preserveScroll: true,
                preserveState: true,
                only: ['entry'],
                onSuccess: (page) => {
                    const next = page.props.entry as Entry;

                    if (
                        next.extracted_body !== null &&
                        next.extracted_body !== ''
                    ) {
                        setOverride('extracted');
                        setManualFailed(false);
                    } else {
                        setManualFailed(true);
                    }
                },
                onError: () => setManualFailed(true),
                onFinish: () => setLoading(false),
            },
        );
    };

    const body = manualFailed ? null : selectBody(entry, override);

    return (
        <>
            <Head title={entry.title ?? 'Reader'} />
            <div className="min-h-svh bg-background">
                <header className="sticky top-0 z-10 flex h-14 items-center px-4 sm:px-6">
                    <Link
                        href={consume().url}
                        aria-label="Back to digest"
                        className="inline-flex size-9 items-center justify-center rounded-xl text-muted-foreground transition-colors duration-200 ease-out outline-none hover:text-foreground focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring focus-visible:outline-solid"
                    >
                        <HugeiconsIcon icon={Cancel01Icon} className="size-5" />
                    </Link>
                </header>

                <ReaderRail
                    sourceUrl={entry.source_url}
                    toggleState={toggleState}
                    onToggleClick={onToggleClick}
                />

                <article className="mx-auto w-full max-w-2xl px-4 pt-8 pb-24 sm:px-6">
                    <header className="mb-8 flex flex-col gap-4">
                        <FeedLine entry={entry} />
                        {entry.title ? (
                            <h1
                                className="text-2xl font-medium tracking-tight text-balance text-foreground"
                                style={{
                                    viewTransitionName: `entry-title-${entry.entry_slug}`,
                                }}
                            >
                                {entry.title}
                            </h1>
                        ) : null}
                        <Byline entry={entry} />
                    </header>

                    {manualFailed && entry.source_url ? (
                        <ManualFailureFallback sourceUrl={entry.source_url} />
                    ) : body ? (
                        <ReaderBody html={body} />
                    ) : null}
                </article>
            </div>
        </>
    );
}

function selectBody(entry: Entry, override: Override): string | null {
    if (override === 'extracted') {
        return entry.extracted_body ?? entry.feed_body;
    }

    if (override === 'feed') {
        return entry.feed_body;
    }

    return entry.auto_choice === 'extracted' && entry.extracted_body
        ? entry.extracted_body
        : entry.feed_body;
}

// Body HTML is sanitized at ingest by HtmlSanitizer (Purify) before it ever
// hits the database; same path the consume page's MicroblogBody renders.
function ReaderBody({ html }: { html: string }) {
    return (
        <div
            className="font-serif text-base leading-relaxed text-foreground [&_a]:text-primary [&_a]:underline-offset-4 [&_a:hover]:underline [&_blockquote]:border-l-2 [&_blockquote]:border-gray-200 [&_blockquote]:pl-4 [&_blockquote]:italic dark:[&_blockquote]:border-gray-700 [&_figcaption]:mt-2 [&_figcaption]:text-sm [&_figcaption]:font-light [&_figcaption]:text-muted-foreground [&_figure]:my-6 [&_h2]:mt-8 [&_h2]:mb-3 [&_h2]:font-sans [&_h3]:mt-6 [&_h3]:mb-2 [&_h3]:font-sans [&_img]:rounded-2xl [&_p]:mb-5 [&_pre]:overflow-x-auto [&_pre]:rounded-xl [&_pre]:bg-gray-100 [&_pre]:p-4 [&_pre]:font-mono [&_pre]:text-sm [&_pre]:not-italic dark:[&_pre]:bg-gray-800"
            dangerouslySetInnerHTML={{ __html: html }}
        />
    );
}

// Manual-fetch failed: the user explicitly asked, so we surface an explicit
// answer — a single CTA. Quiet failure per brand: no toast, no apology copy.
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

function FeedLine({ entry }: { entry: Entry }) {
    return (
        <div className="flex items-center gap-2 font-sans">
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
        <p className="font-sans text-sm font-light text-muted-foreground">
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
