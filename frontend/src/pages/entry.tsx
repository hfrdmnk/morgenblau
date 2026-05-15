import { Cancel01Icon } from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import DOMPurify from 'dompurify';
import { useEffect, useMemo, useState } from 'react';

import { buttonVariants } from '@/components/ui/button-variants';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { PATHS } from '@/lib/paths';
import { safeHref } from '@/lib/utils';

type Entry = {
    id: number;
    title: string | null;
    url: string;
    contentType: string;
    publishedAt: string;
    source: {
        feedUrl: string;
        title: string | null;
        siteUrl: string | null;
    };
    body: string | null;
};

type State =
    | { kind: 'loading' }
    | { kind: 'ok'; entry: Entry }
    | { kind: 'extracting'; entry: Entry }
    | { kind: 'error'; entry: Entry | null };

function entryIDFromLocation(): string | null {
    const params = new URLSearchParams(window.location.search);
    return params.get('id');
}

export function Entry() {
    const id = entryIDFromLocation();
    const [state, setState] = useState<State>({ kind: 'loading' });

    useDocumentTitle(
        state.kind === 'ok' || state.kind === 'extracting'
            ? state.entry.title ?? 'Reader'
            : 'Reader',
    );

    useEffect(() => {
        if (!id) {
            setState({ kind: 'error', entry: null });
            return;
        }
        let cancelled = false;
        const load = async () => {
            try {
                const r = await fetch(`/api/entries/${id}`, {
                    credentials: 'same-origin',
                });
                if (!r.ok) throw new Error(String(r.status));
                const entry = (await r.json()) as Entry;
                if (cancelled) return;

                if (entry.body) {
                    setState({ kind: 'ok', entry });
                    return;
                }

                // Lazy extract — body is null, fetch readability extraction.
                setState({ kind: 'extracting', entry });
                const ex = await fetch(`/api/entries/${id}/extract`, {
                    method: 'POST',
                    credentials: 'same-origin',
                });
                if (!ex.ok) {
                    if (!cancelled) setState({ kind: 'error', entry });
                    return;
                }
                const extracted = (await ex.json()) as Entry;
                if (!cancelled) setState({ kind: 'ok', entry: extracted });
            } catch {
                if (!cancelled) setState({ kind: 'error', entry: null });
            }
        };
        load();
        return () => {
            cancelled = true;
        };
    }, [id]);

    return (
        <div className="min-h-svh bg-card">
            <header className="sticky top-0 z-10 flex h-14 items-center px-4 sm:px-6">
                <a
                    href={PATHS.consume}
                    aria-label="Back to digest"
                    className="inline-flex size-9 items-center justify-center rounded-xl text-muted-foreground transition-colors duration-200 ease-out outline-none hover:text-foreground focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring focus-visible:outline-solid"
                >
                    <HugeiconsIcon icon={Cancel01Icon} className="size-5" />
                </a>
            </header>

            <article className="mx-auto w-full max-w-2xl px-4 pt-8 pb-24 sm:px-6">
                {state.kind === 'loading' && (
                    <p className="text-muted-foreground">Loading…</p>
                )}
                {state.kind === 'extracting' && (
                    <>
                        <EntryHeader entry={state.entry} />
                        <p className="text-muted-foreground">
                            Loading article…
                        </p>
                    </>
                )}
                {state.kind === 'ok' && (
                    <>
                        <EntryHeader entry={state.entry} />
                        {state.entry.body ? (
                            <ReaderBody html={state.entry.body} />
                        ) : (
                            <OpenOriginal url={state.entry.url} />
                        )}
                    </>
                )}
                {state.kind === 'error' && (
                    <>
                        {state.entry && <EntryHeader entry={state.entry} />}
                        <p className="text-muted-foreground">
                            Couldn’t open this entry.
                        </p>
                        {state.entry && <OpenOriginal url={state.entry.url} />}
                    </>
                )}
            </article>
        </div>
    );
}

function EntryHeader({ entry }: { entry: Entry }) {
    const source = entry.source.title ?? entry.source.feedUrl;
    const date = formatDate(entry.publishedAt);
    return (
        <header className="mb-8 flex flex-col gap-4">
            <p className="line-clamp-1 text-sm font-light text-muted-foreground">
                {source}
            </p>
            {entry.title && (
                <h1 className="text-balance text-foreground">{entry.title}</h1>
            )}
            <p className="font-sans text-sm font-light text-muted-foreground">
                {date}
            </p>
            <OpenOriginal url={entry.url} compact />
        </header>
    );
}

// Bodies are server-sanitized (bluemonday UGC policy) at write time per
// SPEC <content-types>. Client also runs DOMPurify as defense-in-depth so the
// render is safe even if a server-side sanitizer regression slips through.
function ReaderBody({ html }: { html: string }) {
    const clean = useMemo(() => DOMPurify.sanitize(html), [html]);
    return (
        <div
            className="font-serif text-base leading-relaxed text-foreground [&_a]:text-primary [&_a]:underline-offset-4 [&_a:hover]:underline [&_blockquote]:border-l-2 [&_blockquote]:border-border [&_blockquote]:pl-4 [&_blockquote]:italic [&_h2]:mt-8 [&_h2]:mb-3 [&_h2]:font-sans [&_h3]:mt-6 [&_h3]:mb-2 [&_h3]:font-sans [&_img]:rounded-2xl [&_ol]:mb-5 [&_ol]:list-decimal [&_ol]:pl-6 [&_p]:mb-5 [&_pre]:overflow-x-auto [&_pre]:rounded-xl [&_pre]:bg-muted [&_pre]:p-4 [&_pre]:font-mono [&_pre]:text-sm [&_ul]:mb-5 [&_ul]:list-disc [&_ul]:pl-6"
            dangerouslySetInnerHTML={{ __html: clean }}
        />
    );
}

function OpenOriginal({
    url,
    compact = false,
}: {
    url: string;
    compact?: boolean;
}) {
    const href = safeHref(url);
    if (!href) return null;
    return (
        <div className={compact ? '' : 'mt-6'}>
            <a
                href={href}
                target="_blank"
                rel="noopener noreferrer"
                className={buttonVariants({ variant: 'secondary' })}
            >
                Open original
            </a>
        </div>
    );
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
