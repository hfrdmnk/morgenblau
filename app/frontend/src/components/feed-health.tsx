import { useState } from 'react';

import {
    Popover,
    PopoverContent,
    PopoverDescription,
    PopoverHeader,
    PopoverTitle,
    PopoverTrigger,
} from '@/components/ui/popover';
import { StatusDot } from '@/components/ui/status-dot';
import { Frame } from '@/design/frame';
import { Text } from '@/design/text';
import type { FeedSource } from '@/hooks/use-sources';

const RELATIVE = new Intl.RelativeTimeFormat('en', { numeric: 'auto' });

export function FeedHealth({
    source,
    labelled = false,
    waitingChannels,
}: {
    source: FeedSource;
    labelled?: boolean;
    waitingChannels?: number;
}) {
    if (source.fetchStatus !== 'unavailable') return null;
    if (source.youtubeOutage) {
        if (labelled) return <YouTubeOutageStatus nextFetchAt={source.nextFetchAt} waitingChannels={waitingChannels} />;
        return (
            <Frame
                as="span"
                aria-label="Waiting on YouTube. Its feeds are down right now."
                direction="row"
                display="inline-flex"
                role="img"
                shrink={false}
            >
                <StatusDot tone="warning" />
            </Frame>
        );
    }
    if (labelled) {
        return (
            <Frame as="span" align="center" direction="row" gap="xs" shrink={false}>
                <StatusDot tone="destructive" />
                <Text as="span">Not fetching</Text>
            </Frame>
        );
    }
    return (
        <Frame
            as="span"
            aria-label="Posts not fetching. Retrying automatically."
            direction="row"
            display="inline-flex"
            role="img"
            shrink={false}
        >
            <StatusDot tone="destructive" />
        </Frame>
    );
}

function YouTubeOutageStatus({ nextFetchAt, waitingChannels = 0 }: { nextFetchAt?: string; waitingChannels?: number }) {
    const [openedAt, setOpenedAt] = useState(0);
    return (
        <Popover
            onOpenChange={(open) => {
                if (open) setOpenedAt(Date.now());
            }}
        >
            <PopoverTrigger variant="explain">
                <StatusDot tone="warning" />
                Waiting on YouTube
            </PopoverTrigger>
            <PopoverContent align="start">
                <PopoverHeader>
                    <PopoverTitle>YouTube’s feeds are down</PopoverTitle>
                    <PopoverDescription pretty>
                        This is on YouTube’s side and usually clears within a few hours, so there’s nothing to fix.
                        Morgenblau checks again {nextCheck(nextFetchAt, openedAt)}, and videos posted in the meantime
                        still arrive.
                    </PopoverDescription>
                </PopoverHeader>
                {waitingChannels > 1 ? (
                    <Text size="xs" tone="subtle">
                        {waitingChannels} of your YouTube channels are waiting.
                    </Text>
                ) : null}
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
    if (generated.health !== 'blocked_by_site')
        note = generated.healthReason ? `Needs attention: ${generated.healthReason}` : 'Needs attention';
    return (
        <Frame paddingTop="xs">
            <Text size="xs" tone="muted">
                {note}
            </Text>
        </Frame>
    );
}

export function SourceHealth({ source }: { source: FeedSource }) {
    return (
        <>
            {source.fetchStatus === 'unavailable' ? (
                <Frame align="center" direction="row" gap="s" paddingTop="m">
                    <FeedHealth source={source} />
                    <Text size="sm" tone="muted">
                        {source.youtubeOutage
                            ? 'YouTube’s feeds are down right now. This is on YouTube’s side, and Morgenblau keeps checking.'
                            : 'Posts aren’t fetching right now. Morgenblau retries automatically.'}
                    </Text>
                </Frame>
            ) : null}
            <GeneratedNote source={source} />
        </>
    );
}
