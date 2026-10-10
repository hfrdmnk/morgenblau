import type { FeedSource } from '@/hooks/use-sources';

export function FeedHealth({ source }: { source: FeedSource }) {
    if (source.fetchStatus !== 'unavailable') return null;
    const label = 'Posts not fetching. Retrying automatically.';
    return (
        <span role="img" aria-label={label} title={label}
            className="inline-block size-2 shrink-0 rounded-full bg-destructive" />
    );
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
