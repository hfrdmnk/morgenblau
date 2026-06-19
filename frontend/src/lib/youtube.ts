// Maps a YouTube channel RSS feed (…/feeds/videos.xml?channel_id=UC…) to the
// uploads-without-Shorts playlist feed (…?playlist_id=UULF…). The UC→UULF swap
// selects YouTube's long-form uploads playlist, which omits Shorts. Returns null
// for any URL that isn't a YouTube channel feed.
export function youtubeShortsFreeFeedUrl(feedUrl: string): string | null {
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
    const channelId = url.searchParams.get('channel_id');
    if (!channelId || !channelId.startsWith('UC')) {
        return null;
    }
    const playlistId = `UULF${channelId.slice(2)}`;
    return `${url.origin}${url.pathname}?playlist_id=${playlistId}`;
}
