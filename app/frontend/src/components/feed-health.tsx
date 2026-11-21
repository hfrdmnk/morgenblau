import { useState } from 'react';

import { Popover, PopoverContent, PopoverDescription, PopoverHeader, PopoverTitle, PopoverTrigger } from '@/components/ui/popover';
import type { FeedSource } from '@/hooks/use-sources';
import { cn } from '@/lib/utils';

const RELATIVE = new Intl.RelativeTimeFormat('en', { numeric: 'auto' });

const Dot = ({ tone }: { tone: 'destructive' | 'warning' }) => (
    <span aria-hidden="true" className={cn('inline-block size-2 shrink-0 rounded-full', tone === 'warning' ? 'bg-warning' : 'bg-destructive')} />
);

// waitingChannels is how many of the reader's YouTube channels wait on the same outage, when the caller knows.
export function FeedHealth({ source, labelled = false, waitingChannels }: { source: FeedSource; labelled?: boolean; waitingChannels?: number }) {
    if (source.fetchStatus !== 'unavailable') return null;
    if (source.youtubeOutage) {
        if (labelled) return <YouTubeOutageStatus nextFetchAt={source.nextFetchAt} waitingChannels={waitingChannels} />;
        return <span role="img" aria-label="Waiting on YouTube. Its feeds are down right now." className="inline-flex shrink-0"><Dot tone="warning" /></span>;
    }
    if (labelled) {
        return <span className="flex shrink-0 items-center gap-1.5"><Dot tone="destructive" />Not fetching</span>;
    }
    return <span role="img" aria-label="Posts not fetching. Retrying automatically." className="inline-flex shrink-0"><Dot tone="destructive" /></span>;
}

function YouTubeOutageStatus({ nextFetchAt, waitingChannels = 0 }: { nextFetchAt?: string; waitingChannels?: number }) {
    // Read when the popover opens, since render must not read the clock.
    const [openedAt, setOpenedAt] = useState(0);
    return (
        <Popover onOpenChange={(open) => { if (open) setOpenedAt(Date.now()); }}>
            <PopoverTrigger className="flex shrink-0 cursor-pointer items-center gap-1.5 rounded-sm underline decoration-subtle-foreground/40 decoration-dotted underline-offset-4 transition-[color] duration-(--motion-duration-fast) hover:text-foreground">
                <Dot tone="warning" />
                Waiting on YouTube
            </PopoverTrigger>
            <PopoverContent align="start">
                <PopoverHeader>
                    <PopoverTitle className="font-medium">YouTube’s feeds are down</PopoverTitle>
                    <PopoverDescription className="text-pretty">
                        This is on YouTube’s side and usually clears within a few hours, so there’s nothing to fix. Morgenblau checks again {nextCheck(nextFetchAt, openedAt)}, and videos posted in the meantime still arrive.
                    </PopoverDescription>
                </PopoverHeader>
                {waitingChannels > 1 ? <p className="text-xs text-subtle-foreground">{waitingChannels} of your YouTube channels are waiting.</p> : null}
            </PopoverContent>
        </Popover>
    );
}

function nextCheck(nextFetchAt: string | undefined, now: number) {
    const minutes = nextFetchAt ? Math.round((Date.parse(nextFetchAt) - now) / 60_000) : 0;
    if (minutes < 1) return 'shortly';
    return minutes < 90 ? RELATIVE.format(minutes, 'minute') : RELATIVE.format(Math.round(minutes / 60), 'hour');
}

export function GeneratedNote({ source }: { source: FeedSource }) {
    const generated = source.generated;
    if (!generated || generated.health === 'ok') return null;
    let note = 'This site blocks feed readers';
    if (generated.health !== 'blocked_by_site') note = generated.healthReason ? `Needs attention: ${generated.healthReason}` : 'Needs attention';
    return <p className="mt-1.5 text-xs text-muted-foreground">{note}</p>;
}

export function SourceHealth({ source }: { source: FeedSource }) {
    return (
        <>
            {source.fetchStatus === 'unavailable' ? (
                <p className="mt-3 flex items-center gap-2.5 text-sm text-muted-foreground">
                    <FeedHealth source={source} />
                    {source.youtubeOutage
                        ? 'YouTube’s feeds are down right now. This is on YouTube’s side, and Morgenblau keeps checking.'
                        : 'Posts aren’t fetching right now. Morgenblau retries automatically.'}
                </p>
            ) : null}
            <GeneratedNote source={source} />
        </>
    );
}
