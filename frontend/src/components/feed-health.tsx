import type { FeedSource } from '@/hooks/use-sources';

export function FeedHealth({ source }: { source: FeedSource }) {
    if (source.fetchStatus !== 'unavailable') return null;
    const label = 'Posts not fetching. Retrying automatically.';
    return (
        <span role="img" aria-label={label} title={label}
            className="inline-block size-2 shrink-0 rounded-full bg-destructive" />
    );
}
