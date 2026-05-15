import { Cancel01Icon, Globe02Icon } from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import type { ReactNode } from 'react';
import { useState } from 'react';

import { ReaderRail } from '@/components/reader-rail';
import type { ExtractedToggleState } from '@/components/reader-rail';
import { buttonVariants } from '@/components/ui/button';
import { useDocumentTitle } from '@/hooks/use-document-title';

type Entry = {
    entry_slug: string;
    title: string | null;
    author: string | null;
    published_at: string | null;
    source_url: string | null;
    source_domain: string | null;
    auto_choice: 'feed' | 'extracted';
    feed: {
        display_title: string;
        favicon_url: string | null;
    };
};

type Override = 'auto' | 'feed' | 'extracted';

const SAMPLE_ENTRY: Entry = {
    entry_slug: 'sample',
    title: 'A quiet square on the open web',
    author: 'Morgenblau',
    published_at: '2026-05-15T07:00:00Z',
    source_url: 'https://morgen.blue/sample',
    source_domain: 'morgen.blue',
    feed: {
        display_title: 'Morgenblau Journal',
        favicon_url: null,
    },
    auto_choice: 'feed',
};

const SAMPLE_BODY: ReactNode = (
    <>
        <p>
            Morgenblau is a calm content platform powered by RSS and the AT
            Protocol. Instead of an infinite feed, you receive a daily digest
            of what you follow — a single edition, gathered while you slept.
        </p>
        <h2>Read what you follow</h2>
        <p>
            Subscriptions live in your atproto repository. Add a feed once and
            every Morgenblau-compatible app sees it.
        </p>
        <blockquote>
            The web is wide and quiet again when you choose what to let in.
        </blockquote>
        <h2>Post what you find</h2>
        <p>
            Share a link with a short note. It lands in the atmosphere as a
            record on your repo, available to other readers.
        </p>
    </>
);

export function Entry() {
    const entry = SAMPLE_ENTRY;
    useDocumentTitle(entry.title ?? 'Reader');
    const [override, setOverride] = useState<Override>('auto');
    const [loading] = useState(false);
    const [manualFailed, setManualFailed] = useState(false);

    const hasExtracted = false;

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

        setManualFailed(true);
    };

    return (
        <div className="min-h-svh bg-card">
            <header className="sticky top-0 z-10 flex h-14 items-center px-4 sm:px-6">
                <a
                    href="/consume"
                    aria-label="Back to digest"
                    className="inline-flex size-9 items-center justify-center rounded-xl text-muted-foreground transition-colors duration-200 ease-out outline-none hover:text-foreground focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring focus-visible:outline-solid"
                >
                    <HugeiconsIcon icon={Cancel01Icon} className="size-5" />
                </a>
            </header>

            <ReaderRail
                sourceUrl={entry.source_url}
                extractedToggle={{
                    state: toggleState,
                    onClick: onToggleClick,
                }}
            />

            <article className="mx-auto w-full max-w-2xl px-4 pt-8 pb-24 sm:px-6">
                <header className="mb-8 flex flex-col gap-4">
                    <FeedLine entry={entry} />
                    {entry.title ? (
                        <h1 className="text-2xl font-medium tracking-tight text-balance text-foreground">
                            {entry.title}
                        </h1>
                    ) : null}
                    <Byline entry={entry} />
                </header>

                {manualFailed && entry.source_url ? (
                    <ManualFailureFallback sourceUrl={entry.source_url} />
                ) : (
                    <ReaderBody>{SAMPLE_BODY}</ReaderBody>
                )}
            </article>
        </div>
    );
}

function ReaderBody({ children }: { children: ReactNode }) {
    return (
        <div className="font-serif text-base leading-relaxed text-foreground [&_a]:text-primary [&_a]:underline-offset-4 [&_a:hover]:underline [&_blockquote]:border-l-2 [&_blockquote]:border-gray-200 [&_blockquote]:pl-4 [&_blockquote]:italic dark:[&_blockquote]:border-gray-700 [&_figcaption]:mt-2 [&_figcaption]:text-sm [&_figcaption]:font-light [&_figcaption]:text-muted-foreground [&_figure]:my-6 [&_h2]:mt-8 [&_h2]:mb-3 [&_h2]:font-sans [&_h3]:mt-6 [&_h3]:mb-2 [&_h3]:font-sans [&_img]:rounded-2xl [&_li]:scroll-mt-20 [&_ol]:mb-5 [&_ol]:list-decimal [&_ol]:pl-6 [&_p]:mb-5 [&_pre]:overflow-x-auto [&_pre]:rounded-xl [&_pre]:bg-gray-100 [&_pre]:p-4 [&_pre]:font-mono [&_pre]:text-sm [&_pre]:not-italic dark:[&_pre]:bg-gray-800 [&_sub]:font-sans [&_sub]:text-xs [&_sup]:scroll-mt-20 [&_sup]:font-sans [&_sup]:text-xs [&_ul]:mb-5 [&_ul]:list-disc [&_ul]:pl-6">
            {children}
        </div>
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
