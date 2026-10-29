import type { FeedSource } from '@/hooks/use-sources';

export function FeedHealth({ source, labelled = false }: { source: FeedSource; labelled?: boolean }) {
    if (source.fetchStatus !== 'unavailable') return null;
    const dot = <span aria-hidden="true" className="inline-block size-2 shrink-0 rounded-full bg-destructive" />;
    if (labelled) {
        return <span className="flex shrink-0 items-center gap-1.5">{dot}Not fetching</span>;
    }
    return <span role="img" aria-label="Posts not fetching. Retrying automatically." className="inline-flex shrink-0">{dot}</span>;
}

export function GeneratedNote({ source }: { source: FeedSource }) {
    const generated = source.generated;
    if (!generated) return null;
    let note = 'Generated feed';
    if (generated.health === 'blocked_by_site') note = 'This site blocks feed readers';
    else if (generated.health !== 'ok') {
        note += ' · Needs attention';
        if (generated.healthReason) note += `: ${generated.healthReason}`;
    }
    return <p className="mt-1.5 text-xs text-muted-foreground">{note}</p>;
}

export function SourceHealth({ source }: { source: FeedSource }) {
    return (
        <>
            {source.fetchStatus === 'unavailable' ? (
                <p className="mt-3 flex items-center gap-2.5 text-sm text-muted-foreground">
                    <FeedHealth source={source} />
                    Posts aren’t fetching right now. Morgenblau retries automatically.
                </p>
            ) : null}
            <GeneratedNote source={source} />
        </>
    );
}
