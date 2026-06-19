// Parses a YouTube uploads-feed URL (…/feeds/videos.xml on a youtube.com host).
function parseYoutubeFeed(feedUrl: string): URL | null {
    let url: URL;
    try {
        url = new URL(feedUrl);
    } catch {
        return null;
    }
    const host = url.hostname.toLowerCase();
    if (host !== 'youtube.com' && !host.endsWith('.youtube.com')) {
        return null;
    }
    if (url.pathname !== '/feeds/videos.xml') {
        return null;
    }
    return url;
}

// Maps a YouTube channel feed (…?channel_id=UC…) to the uploads-without-Shorts
// playlist feed (…?playlist_id=UULF…). The UC→UULF swap selects the long-form
// uploads playlist, which omits Shorts. Returns null when feedUrl isn't a channel feed.
export function youtubeShortsFreeFeedUrl(feedUrl: string): string | null {
    const url = parseYoutubeFeed(feedUrl);
    if (!url) {
        return null;
    }
    const channelId = url.searchParams.get('channel_id');
    if (!channelId || !channelId.startsWith('UC')) {
        return null;
    }
    return `${url.origin}${url.pathname}?playlist_id=UULF${channelId.slice(2)}`;
}

// Normalizes a YouTube uploads feed — channel form or shorts-free playlist form —
// back to the canonical channel feed. Inverse of youtubeShortsFreeFeedUrl. Returns
// null when feedUrl isn't a YouTube uploads feed.
export function youtubeChannelFeedUrl(feedUrl: string): string | null {
    const url = parseYoutubeFeed(feedUrl);
    if (!url) {
        return null;
    }
    const channelId = url.searchParams.get('channel_id');
    if (channelId && channelId.startsWith('UC')) {
        return `${url.origin}${url.pathname}?channel_id=${channelId}`;
    }
    const playlistId = url.searchParams.get('playlist_id');
    if (playlistId && playlistId.startsWith('UULF')) {
        return `${url.origin}${url.pathname}?channel_id=UC${playlistId.slice(4)}`;
    }
    return null;
}

// True when feedUrl is the uploads-without-Shorts playlist feed (…?playlist_id=UULF…).
export function isYoutubeShortsFreeFeedUrl(feedUrl: string): boolean {
    const url = parseYoutubeFeed(feedUrl);
    if (!url) {
        return false;
    }
    const playlistId = url.searchParams.get('playlist_id');
    return playlistId !== null && playlistId.startsWith('UULF');
}
