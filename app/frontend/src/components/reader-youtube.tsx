import { useEffect, useMemo, useRef, useState, type RefObject } from 'react';

import { PostList } from '@/components/ui/post-list';
import { TranscriptList, TranscriptPassage } from '@/components/ui/transcript';
import { PlayerSlot, VideoEmbed } from '@/components/ui/video';
import { Frame } from '@/design/frame';
import { Text } from '@/design/text';
import { useHeightProperty } from '@/hooks/use-height-property';
import { useReaderFont } from '@/hooks/use-reader-font';
import { useYouTubePlayer, youtubeEmbedURL, type YouTubePlayer } from '@/hooks/use-youtube-player';
import { api } from '@/lib/api';
import { activeIndex, formatTimestamp, groupCues, type Transcript, type TranscriptParagraph } from '@/lib/transcript';

export function ReaderYouTube({ entrySlug, videoID, title }: { entrySlug: string; videoID: string; title: string }) {
    const frame = useRef<HTMLIFrameElement>(null);
    const playerBox = useRef<HTMLDivElement>(null);
    const player = useYouTubePlayer(frame);
    useHeightProperty(playerBox, '--reader-player-height');
    const transcript = useTranscript(entrySlug);
    return (
        <Frame gap="4xl" paddingBottom="4xl">
            <PlayerSlot playing={player.playing} ref={playerBox}>
                <VideoEmbed
                    ref={frame}
                    src={youtubeEmbedURL(videoID)}
                    title={title}
                    allow="accelerometer; autoplay; encrypted-media; gyroscope; picture-in-picture; fullscreen"
                    allowFullScreen
                    referrerPolicy="strict-origin-when-cross-origin"
                />
            </PlayerSlot>
            {transcript?.status === 'ready' ? (
                <ReaderTranscript
                    kind={transcript.kind}
                    paragraphs={groupCues(transcript.cues)}
                    player={player}
                    playerBox={playerBox}
                />
            ) : null}
        </Frame>
    );
}

function useTranscript(entrySlug: string): Transcript | null {
    const [transcript, setTranscript] = useState<Transcript | null>(null);
    useEffect(() => {
        const controller = new AbortController();
        api<Transcript>(`/api/entries/${encodeURIComponent(entrySlug)}/transcript`, {
            method: 'POST',
            signal: controller.signal,
        }).then(setTranscript, () => undefined);
        return () => controller.abort();
    }, [entrySlug]);
    return transcript;
}

function ReaderTranscript({
    paragraphs,
    kind,
    player,
    playerBox,
}: {
    paragraphs: TranscriptParagraph[];
    kind: 'manual' | 'asr';
    player: YouTubePlayer;
    playerBox: RefObject<HTMLDivElement | null>;
}) {
    const [font] = useReaderFont();
    const list = useRef<HTMLOListElement>(null);
    const started = player.playing || player.currentTimeMs > 0;
    const active = useMemo(
        () => (started ? activeIndex(paragraphs, player.currentTimeMs) : -1),
        [paragraphs, started, player.currentTimeMs],
    );
    const previous = useRef(active);
    useEffect(() => {
        const before = previous.current;
        previous.current = active;
        if (player.playing && list.current && active >= 0)
            follow(list.current.children, before, active, playerBox.current);
    }, [active, player.playing, playerBox]);
    return (
        <Frame aria-labelledby="transcript-title" as="section" gap="2xl">
            <Text as="h2" id="transcript-title" size="sm" tone="muted">
                <Frame align="baseline" as="span" direction="row" gap="m">
                    <Text as="span" tone="foreground" weight="medium">
                        Transcript
                    </Text>
                    {kind === 'asr' ? <Text as="span">Auto-generated</Text> : null}
                </Frame>
            </Text>
            <PostList arrowsOnly>
                <TranscriptList font={font} ref={list} stuck={player.playing}>
                    {paragraphs.map((paragraph, index) => (
                        <TranscriptPassage
                            current={index === active}
                            key={paragraph.start}
                            label={`Play from ${formatTimestamp(paragraph.start)}`}
                            onPlay={player.ready ? () => player.seekTo(paragraph.start) : undefined}
                            time={formatTimestamp(paragraph.start)}
                        >
                            {paragraph.text}
                        </TranscriptPassage>
                    ))}
                </TranscriptList>
            </PostList>
        </Frame>
    );
}

function follow(items: HTMLCollection, before: number, active: number, stuck: HTMLElement | null) {
    if (before === active) return uncover(items[active], stuck);
    if (before < 0 || isOnScreen(items[before], stuck)) reveal(items[active], stuck);
}

function reveal(item: Element, playerBox: HTMLElement | null) {
    const covered = playerBox?.offsetHeight ?? 0;
    const free = window.innerHeight - covered;
    const rect = item.getBoundingClientRect();
    const offset = covered + Math.max(0, (free - rect.height) / 2);
    const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    window.scrollTo({ top: window.scrollY + rect.top - offset, behavior: reduce ? 'auto' : 'smooth' });
}

function uncover(item: Element, stuck: HTMLElement | null) {
    if (stuck && item.getBoundingClientRect().top < stuck.getBoundingClientRect().bottom) reveal(item, stuck);
}

function isOnScreen(element: Element, stuck: HTMLElement | null): boolean {
    const rect = element.getBoundingClientRect();
    const top = stuck ? Math.max(0, stuck.getBoundingClientRect().bottom) : 0;
    return rect.bottom > top && rect.top < window.innerHeight;
}
