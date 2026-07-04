import { describe, expect, test } from 'bun:test';

import {
    isYoutubeShortsFreeFeedUrl,
    youtubeChannelFeedUrl,
    youtubeShortsFreeFeedUrl,
} from './youtube';

const channel = (id: string) =>
    `https://www.youtube.com/feeds/videos.xml?channel_id=${id}`;
const playlist = (id: string) =>
    `https://www.youtube.com/feeds/videos.xml?playlist_id=${id}`;

describe('youtubeShortsFreeFeedUrl', () => {
    test('maps a UC channel feed to its UULF uploads playlist', () => {
        expect(youtubeShortsFreeFeedUrl(channel('UCabc123'))).toBe(
            playlist('UULFabc123'),
        );
    });

    test('returns null for a non-UC channel id', () => {
        expect(youtubeShortsFreeFeedUrl(channel('HCabc123'))).toBeNull();
    });

    test('returns null for a non-YouTube host', () => {
        expect(
            youtubeShortsFreeFeedUrl(
                'https://example.com/feeds/videos.xml?channel_id=UCabc123',
            ),
        ).toBeNull();
    });

    test('returns null for the wrong path', () => {
        expect(
            youtubeShortsFreeFeedUrl(
                'https://www.youtube.com/watch?channel_id=UCabc123',
            ),
        ).toBeNull();
    });
});

describe('youtubeChannelFeedUrl', () => {
    test('round-trips a UULF playlist feed back to its UC channel feed', () => {
        const shortsFree = youtubeShortsFreeFeedUrl(channel('UCabc123'));
        expect(shortsFree).not.toBeNull();
        expect(youtubeChannelFeedUrl(shortsFree as string)).toBe(
            channel('UCabc123'),
        );
    });

    test('passes a UC channel feed through unchanged', () => {
        expect(youtubeChannelFeedUrl(channel('UCabc123'))).toBe(
            channel('UCabc123'),
        );
    });

    test('returns null for a non-UULF playlist id', () => {
        expect(youtubeChannelFeedUrl(playlist('PLabc123'))).toBeNull();
    });

    test('returns null for a non-YouTube host', () => {
        expect(youtubeChannelFeedUrl('https://example.com/feed')).toBeNull();
    });
});

describe('isYoutubeShortsFreeFeedUrl', () => {
    test('true only for the UULF playlist form', () => {
        expect(isYoutubeShortsFreeFeedUrl(playlist('UULFabc123'))).toBe(true);
    });

    test('false for a plain UC channel feed', () => {
        expect(isYoutubeShortsFreeFeedUrl(channel('UCabc123'))).toBe(false);
    });

    test('false for a non-UULF playlist feed', () => {
        expect(isYoutubeShortsFreeFeedUrl(playlist('PLabc123'))).toBe(false);
    });

    test('false for a non-YouTube host', () => {
        expect(isYoutubeShortsFreeFeedUrl('https://example.com/feed')).toBe(
            false,
        );
    });
});
