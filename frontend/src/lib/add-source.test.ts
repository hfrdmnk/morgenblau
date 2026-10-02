import { afterEach, expect, test } from 'bun:test';

import {
    addSource,
    discoverSources,
    newsletterAddress,
    saveSources,
    sourceKey,
    sourceLocation,
    sourceTitle,
    subscriptionChanges,
} from './add-source';

const realFetch = globalThis.fetch;
afterEach(() => {
    globalThis.fetch = realFetch;
});

function respond(handler: (url: string, init?: RequestInit) => Response) {
    globalThis.fetch = Object.assign(
        async (url: Parameters<typeof fetch>[0], init?: RequestInit) =>
            handler(String(url), init),
        {
            preconnect: realFetch.preconnect,
        },
    );
}

test('discovery trims input, accepts bare domains, and passes cancellation', async () => {
    const signal = new AbortController().signal;
    respond((url, init) => {
        expect(url).toBe('/api/subscriptions/resolve');
        expect(JSON.parse(String(init?.body))).toEqual({
            url: 'https://example.com/posts',
        });
        expect(init?.signal).toBe(signal);
        return Response.json({ candidates: [], existingSubscriptions: [] });
    });
    await discoverSources('  example.com/posts  ', signal);
});

test('discovery rejects non-web schemes without sending a request', async () => {
    respond(() => {
        throw new Error('must not request');
    });
    await expect(
        discoverSources('javascript:alert(1)', new AbortController().signal),
    ).rejects.toThrow('Enter a website or feed URL.');
});

test('keeps RSS titles but does not customize discovered native publications', async () => {
    const bodies: unknown[] = [];
    respond((_url, init) => {
        bodies.push(JSON.parse(String(init?.body)));
        return Response.json({ rkey: '3example' });
    });
    await addSource({ feedUrl: 'https://example.com/rss', title: 'Example' });
    await addSource({
        kind: 'standardfeed',
        publication: 'at://did:plc:example/site.standard.publication/test',
        title: 'Native',
        siteUrl: 'https://example.org',
    });
    expect(bodies).toEqual([
        { feedUrl: 'https://example.com/rss', title: 'Example' },
        {
            publication: 'at://did:plc:example/site.standard.publication/test',
            siteUrl: 'https://example.org',
        },
    ]);
});

test('notifies after each confirmed addition, including before a partial failure', async () => {
    let completed = 0;
    const observed: number[] = [];
    const listener = () => observed.push(completed);
    subscriptionChanges.addEventListener('change', listener);
    respond(() => {
        expect(observed.length).toBe(completed);
        completed++;
        return completed === 3
            ? Response.json({ message: 'Unavailable' }, { status: 502 })
            : Response.json({ rkey: '3example' });
    });
    try {
        await addSource({ feedUrl: 'https://example.com/first.xml' });
        await addSource({ feedUrl: 'https://example.com/second.xml' });
        await expect(
            addSource({ feedUrl: 'https://example.com/third.xml' }),
        ).rejects.toThrow('Unavailable');
        expect(observed).toEqual([1, 2]);
    } finally {
        subscriptionChanges.removeEventListener('change', listener);
    }
});

test('does not notify when the subscription response cannot be confirmed', async () => {
    let notifications = 0;
    const listener = () => notifications++;
    subscriptionChanges.addEventListener('change', listener);
    respond(() => {
        throw new TypeError('Network failure');
    });
    try {
        await expect(
            addSource({ feedUrl: 'https://example.com/rss' }),
        ).rejects.toThrow('Network failure');
        expect(notifications).toBe(0);
    } finally {
        subscriptionChanges.removeEventListener('change', listener);
    }
});

test('save confirms each source sequentially and stops at a partial failure', async () => {
    const candidates = ['first', 'second', 'third'].map((name) => ({
        feedUrl: `https://example.com/${name}.xml`,
    }));
    const requested: string[] = [];
    const confirmed: string[] = [];
    respond((_url, init) => {
        expect(confirmed).toEqual(requested);
        const { feedUrl } = JSON.parse(String(init?.body));
        requested.push(feedUrl);
        return requested.length === 2
            ? Response.json({ message: 'Unavailable' }, { status: 502 })
            : Response.json({ rkey: '3example' });
    });
    await expect(
        saveSources(candidates, (candidate) => {
            confirmed.push(sourceKey(candidate));
        }),
    ).rejects.toThrow('Unavailable');
    expect(requested).toEqual([
        'https://example.com/first.xml',
        'https://example.com/second.xml',
    ]);
    expect(confirmed).toEqual(['https://example.com/first.xml']);
});

test('untitled sources prefer the site for their title but the feed for their location', () => {
    const feed = {
        feedUrl: 'https://feeds.example.com/rss.xml',
        siteUrl: 'https://example.org',
    };
    expect(sourceKey(feed)).toBe('https://feeds.example.com/rss.xml');
    expect(sourceTitle(feed)).toBe('https://example.org');
    expect(sourceLocation(feed)).toBe('https://feeds.example.com/rss.xml');

    const native = {
        kind: 'standardfeed' as const,
        publication: 'at://did:plc:reader/site.standard.publication/journal',
    };
    expect(sourceKey(native)).toBe(native.publication);
    expect(sourceTitle(native)).toBe(native.publication);
    expect(sourceLocation(native)).toBe(native.publication);
    expect(sourceTitle({ ...native, title: 'Journal' })).toBe('Journal');
});

test('newsletter address reuses an existing address without a mutation', async () => {
    respond((_url, init) => {
        expect(init?.method).toBe('GET');
        return Response.json({ address: 'reader@news.example.com' });
    });
    expect(await newsletterAddress(new AbortController().signal)).toBe(
        'reader@news.example.com',
    );
});

test('a missing address does not make the dialog provision an account', async () => {
    const methods: unknown[] = [];
    respond((_url, init) => {
        methods.push(init?.method);
        return Response.json(
            init?.method === 'POST' ? { address: 'new@news.example.com' } : {},
        );
    });
    expect(
        await newsletterAddress(new AbortController().signal),
    ).toBeUndefined();
    expect(methods).toEqual(['GET']);
});

test('newsletter address lookup passes cancellation', async () => {
    const signal = new AbortController().signal;
    respond((url, init) => {
        expect(url).toBe('/api/newsletters/address');
        expect(init?.signal).toBe(signal);
        return Response.json({ address: 'new@news.example.com' });
    });
    expect(await newsletterAddress(signal)).toBe('new@news.example.com');
});

test('newsletter read failure does not attempt to create an address', async () => {
    const methods: unknown[] = [];
    respond((_url, init) => {
        methods.push(init?.method);
        return Response.json({ message: 'Unavailable' }, { status: 503 });
    });
    await expect(
        newsletterAddress(new AbortController().signal),
    ).rejects.toThrow('Unavailable');
    expect(methods).toEqual(['GET']);
});

test('subscription permission failure preserves the reauthentication signal', async () => {
    respond(() =>
        Response.json(
            { code: 'reauth_required', message: 'Sign in again' },
            { status: 403 },
        ),
    );
    await expect(
        addSource({ feedUrl: 'https://example.com/rss' }),
    ).rejects.toMatchObject({ isReauth: true });
});
