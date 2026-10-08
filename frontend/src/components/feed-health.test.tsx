import { expect, test } from 'bun:test';
import { renderToStaticMarkup } from 'react-dom/server';

import type { FeedSource } from '@/hooks/use-sources';

import { FeedHealth } from './feed-health';

const source: FeedSource = {
    rkey: 'example', feedUrl: 'https://publication.example.com/feed.xml', primary: false,
    fetchStatus: 'unavailable', lastFetchedAt: '2026-10-08T07:00:00Z', nextFetchAt: '2026-10-08T08:05:00Z',
};

test('a failed refresh shows a red dot even when the feed has an older successful check', () => {
    const html = renderToStaticMarkup(<FeedHealth source={source} />);
    expect(html).toContain('role="img"');
    expect(html).toContain('aria-label="Posts not fetching. Retrying automatically."');
    expect(html).toContain('bg-destructive');
    expect(html).not.toContain('bg-success');
    expect(html).not.toContain('<p');
    expect(html).not.toContain('<time');
});

test('recovery removes the dot, and an unchecked feed never shows one', () => {
    const ready = renderToStaticMarkup(<FeedHealth source={{ ...source, fetchStatus: 'ready', nextFetchAt: undefined }} />);
    expect(ready).toBe('');
    const waiting = renderToStaticMarkup(<FeedHealth source={{ ...source, fetchStatus: 'waiting', lastFetchedAt: undefined }} />);
    expect(waiting).toBe('');
});
