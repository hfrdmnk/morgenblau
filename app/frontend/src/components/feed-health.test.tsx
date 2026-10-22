import { expect, test } from 'bun:test';
import { renderToStaticMarkup } from 'react-dom/server';

import type { FeedSource } from '@/hooks/use-sources';

import { FeedHealth, GeneratedNote } from './feed-health';

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

test('a generated feed is marked quietly and says when it needs attention', () => {
    const generated = { ...source, feedUrl: 'https://app.example.com/feed/abc', fetchStatus: 'ready' as const };
    expect(renderToStaticMarkup(<GeneratedNote source={source} />)).toBe('');
    expect(renderToStaticMarkup(<GeneratedNote source={{ ...generated, generated: { health: 'ok', healthReason: '' } }} />))
        .toContain('>Generated feed</p>');
    const unsure = renderToStaticMarkup(
        <GeneratedNote source={{ ...generated, generated: { health: 'needs_attention', healthReason: 'fewer posts than before' } }} />,
    );
    expect(unsure).toContain('Generated feed · Needs attention: fewer posts than before');
    expect(renderToStaticMarkup(<GeneratedNote source={{ ...generated, generated: { health: 'broken', healthReason: '' } }} />))
        .toContain('Generated feed · Needs attention</p>');
    expect(renderToStaticMarkup(<GeneratedNote source={{ ...generated, generated: { health: 'blocked_by_site', healthReason: 'challenge page' } }} />))
        .toContain('>This site blocks feed readers</p>');
    expect(unsure).not.toContain('text-destructive');
});
