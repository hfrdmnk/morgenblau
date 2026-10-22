import { describe, expect, test } from 'bun:test';

import { hasNewReaderContent, httpURL, readerBody, readerContentURL, readerMetadata, youtubeID } from './reader';

test('newsletter body keeps private app asset URLs and blocked images unchanged', () => {
    const body = '<img src="/api/newsletter-assets/example-id"><img alt="Blocked image">';
    expect(readerBody({
        entrySlug: 'newsletter-example',
        title: 'Example newsletter',
        url: 'https://publisher.example/newsletter',
        contentType: 'newsletter',
        publishedAt: '2026-09-30T09:00:00Z',
        source: { title: 'Example Letters', siteUrl: null },
        body,
    })).toBe(body);
});

test('feed content URLs resolve against the article without moving local fragments', () => {
    const base = 'https://publisher.example/posts/morning';
    expect(readerContentURL('/images/chart.png', base)).toBe('https://publisher.example/images/chart.png');
    expect(readerContentURL('next', base)).toBe('https://publisher.example/posts/next');
    expect(readerContentURL('../images/chart.png', base)).toBe('https://publisher.example/images/chart.png');
    expect(readerContentURL('//media.example/chart.png', base)).toBe('https://media.example/chart.png');
    expect(readerContentURL('#section', base)).toBe('#section');
    expect(readerContentURL('mailto:writer@example.com', base)).toBe('mailto:writer@example.com');
    expect(readerContentURL('https://other.example/article', base)).toBe('https://other.example/article');
});

test('extraction must return nonempty content different from the existing body', () => {
    const summary = '<p>A short summary.</p>';
    expect(hasNewReaderContent(summary, summary)).toBe(false);
    expect(hasNewReaderContent(summary, ` ${summary}\n`)).toBe(false);
    expect(hasNewReaderContent(summary, ' ')).toBe(false);
    expect(hasNewReaderContent(summary, null)).toBe(false);
    expect(hasNewReaderContent(null, '<p>Full article.</p>')).toBe(true);
    expect(hasNewReaderContent(summary, '<p>A longer article.</p>')).toBe(true);
});

describe('reader source URLs', () => {
    test('only allows absolute web URLs', () => {
        expect(httpURL('https://publisher.example/article')).toBe('https://publisher.example/article');
        for (const url of ['javascript:alert(1)', 'data:text/html,hi', '//publisher.example', '', undefined]) {
            expect(httpURL(url)).toBeNull();
        }
    });

    test('recognizes YouTube watch, short, and embed links on exact hosts', () => {
        for (const url of [
            'https://www.youtube.com/watch?v=AbCdEf123_-&t=20',
            'https://youtu.be/AbCdEf123_-',
            'https://m.youtube.com/shorts/AbCdEf123_-',
            'https://www.youtube-nocookie.com/embed/AbCdEf123_-',
        ]) expect(youtubeID(url)).toBe('AbCdEf123_-');
        for (const url of [
            'https://youtube.com.evil.example/watch?v=AbCdEf123_-',
            'https://notyoutube.com/watch?v=AbCdEf123_-',
            'https://youtube.com/watch?v=bad',
            'https://youtube.com/channel/AbCdEf123_-',
            'javascript:alert(1)',
        ]) expect(youtubeID(url)).toBeNull();
    });
});

test('metadata tolerates malformed JSON and rejects non-video or unsafe enclosures', () => {
    expect(readerMetadata('{')).toEqual({});
    expect(readerMetadata('null')).toEqual({});
    expect(readerMetadata('{"author":42,"enclosure":{"url":"javascript:alert(1)","type":"video/mp4"}}')).toEqual({});
    expect(readerMetadata('{"enclosure":{"url":"https://media.example/a.mp3","type":"audio/mpeg"}}')).toEqual({});
    expect(readerMetadata('{"author":"Example Writer","enclosure":{"url":"https://media.example/a.mp4","type":"video/mp4"}}')).toEqual({
        author: 'Example Writer', video: { url: 'https://media.example/a.mp4', type: 'video/mp4' },
    });
    expect(readerMetadata('{"enclosure":{"url":"https://media.example/a.mp4","type":"Video/MP4"}}')).toEqual({
        video: { url: 'https://media.example/a.mp4', type: 'video/mp4' },
    });
});
