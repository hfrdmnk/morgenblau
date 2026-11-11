import { useEffect, useState, type RefObject } from 'react';

// The embed speaks the postMessage protocol of YouTube's iframe_api script; driving it directly keeps the embed the only third-party resource.
export const PLAYER_ORIGIN = 'https://www.youtube-nocookie.com';
const HANDSHAKE_EVERY_MS = 250;
const HANDSHAKE_TRIES = 40;
const PLAYING = 1;

export type YouTubePlayer = {
    ready: boolean;
    playing: boolean;
    // Milliseconds into the video, as last reported by the player.
    currentTime: number;
    seekTo: (ms: number) => void;
};

export function youtubeEmbedURL(videoID: string): string {
    return `${PLAYER_ORIGIN}/embed/${videoID}?enablejsapi=1&origin=${encodeURIComponent(location.origin)}`;
}

export function useYouTubePlayer(iframe: RefObject<HTMLIFrameElement | null>): YouTubePlayer {
    const [ready, setReady] = useState(false);
    const [playing, setPlaying] = useState(false);
    const [currentTime, setCurrentTime] = useState(0);

    useEffect(() => {
        const frame = iframe.current;
        if (!frame) return;
        let heard = false;
        let tries = 0;
        let timer: ReturnType<typeof setInterval> | undefined;
        // The player ignores messages until its own script has loaded, so the handshake repeats until it answers.
        const handshake = () => {
            clearInterval(timer);
            tries = 0;
            timer = setInterval(() => {
                if (heard || ++tries > HANDSHAKE_TRIES) return clearInterval(timer);
                post(frame, { event: 'listening' });
                post(frame, { event: 'command', func: 'addEventListener', args: ['onStateChange'] });
            }, HANDSHAKE_EVERY_MS);
        };
        const onMessage = (event: MessageEvent) => {
            if (event.origin !== PLAYER_ORIGIN || event.source !== frame.contentWindow) return;
            const update = readPlayerMessage(event.data);
            if (!update) return;
            heard = true;
            setReady(true);
            if (update.time !== undefined) setCurrentTime(update.time);
            if (update.playing !== undefined) setPlaying(update.playing);
        };
        window.addEventListener('message', onMessage);
        frame.addEventListener('load', handshake);
        handshake();
        return () => {
            clearInterval(timer);
            window.removeEventListener('message', onMessage);
            frame.removeEventListener('load', handshake);
        };
    }, [iframe]);

    const seekTo = (ms: number) => {
        post(iframe.current, { event: 'command', func: 'seekTo', args: [ms / 1000, true] });
        post(iframe.current, { event: 'command', func: 'playVideo', args: [] });
        setCurrentTime(ms);
    };

    return { ready, playing, currentTime, seekTo };
}

function post(frame: HTMLIFrameElement | null, message: object) {
    frame?.contentWindow?.postMessage(JSON.stringify({ ...message, id: 1, channel: 'widget' }), PLAYER_ORIGIN);
}

type PlaybackUpdate = { time?: number; playing?: boolean };

// What a player message says about playback; null for anything that is not one.
function readPlayerMessage(raw: unknown): PlaybackUpdate | null {
    const data = parseMessage(raw);
    if (!data) return null;
    if (data.event === 'onStateChange') return typeof data.info === 'number' ? { playing: data.info === PLAYING } : {};
    return data.event === 'infoDelivery' ? readInfo(data.info) : {};
}

function parseMessage(raw: unknown): { event?: string; info?: unknown } | null {
    if (typeof raw !== 'string') return null;
    try {
        return JSON.parse(raw);
    } catch {
        return null;
    }
}

function readInfo(info: unknown): PlaybackUpdate {
    const { currentTime, playerState } = (info ?? {}) as { currentTime?: unknown; playerState?: unknown };
    return {
        time: typeof currentTime === 'number' ? Math.round(currentTime * 1000) : undefined,
        playing: typeof playerState === 'number' ? playerState === PLAYING : undefined,
    };
}
