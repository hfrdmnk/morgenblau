import { afterEach, expect, test } from 'bun:test';

import { ApiError } from './api';
import {
    addSource,
    anchorError,
    anchorRejection,
    canRetryGeneration,
    discoverSources,
    generatedCandidate,
    generatedVerdict,
    generateSource,
    generationError,
    newsletterAddress,
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

test('keeps RSS titles, sends options only when set, and does not otherwise customize native publications', async () => {
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
    await addSource(
        { kind: 'standardfeed', publication: 'at://did:plc:example/site.standard.publication/tagged' },
        { tags: ['Friends'], primary: true },
    );
    await addSource({ feedUrl: 'https://example.com/plain' }, { tags: [], primary: false });
    expect(bodies).toEqual([
        { feedUrl: 'https://example.com/rss', title: 'Example' },
        {
            publication: 'at://did:plc:example/site.standard.publication/test',
            siteUrl: 'https://example.org',
        },
        {
            publication: 'at://did:plc:example/site.standard.publication/tagged',
            tags: ['Friends'],
            primary: true,
        },
        { feedUrl: 'https://example.com/plain' },
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

const generated = {
    id: 'abc',
    feedUrl: 'https://app.example.com/feed/abc',
    title: 'Example Publication',
    siteUrl: 'https://news.example.com/',
    health: 'ok' as const,
    healthReason: '',
    needsAnchor: false,
    items: [
        { url: 'https://news.example.com/posts/first', title: 'First post', date: '2026-10-09T08:00:00Z' },
    ],
    itemCount: 12,
};

test('generating a feed posts the page, and the post the reader pointed at', async () => {
    const signal = new AbortController().signal;
    const bodies: unknown[] = [];
    respond((url, init) => {
        expect(url).toBe('/api/web-sources');
        expect(init?.method).toBe('POST');
        expect(init?.signal).toBe(signal);
        bodies.push(JSON.parse(String(init?.body)));
        return Response.json(generated);
    });
    expect(await generateSource('https://news.example.com/', undefined, signal)).toEqual(generated);
    await generateSource('https://news.example.com/', ' news.example.com/posts/first ', signal);
    expect(bodies).toEqual([
        { url: 'https://news.example.com/' },
        { url: 'https://news.example.com/', anchorUrl: 'https://news.example.com/posts/first' },
    ]);
});

test('a pointed-at post must be a web address before anything is generated', async () => {
    respond(() => {
        throw new Error('must not request');
    });
    await expect(
        generateSource('https://news.example.com/', 'javascript:alert(1)', new AbortController().signal),
    ).rejects.toThrow('Enter the address of one post on that page.');
});

test('generation failures read calmly by cause, in house copy', () => {
    const limited = new ApiError(429, 'rate_limited', 'You have generated a lot of feeds.');
    expect(generationError(limited)).toBe('You’ve generated a lot of feeds recently. Try again later.');
    expect(generationError(new ApiError(403, 'rate_limited', 'Daily cap reached'))).toBe(
        'You’ve generated a lot of feeds recently. Try again later.',
    );
    expect(generationError(new ApiError(502, 'upstream_error', "Couldn't read that page"))).toBe(
        'Couldn’t read that page.',
    );
    expect(generationError(new ApiError(503, 'unavailable', "Feed generation isn't available yet"))).toBe(
        'Feed generation isn’t available right now.',
    );
    expect(generationError(new ApiError(500, 'internal_error', 'internal error'))).toBe(
        'Couldn’t generate a feed. Try again.',
    );
    expect(generationError(new ApiError(400, 'invalid_request', 'Enter a web address that starts with http:// or https://'))).toBe(
        'Enter a website URL.',
    );
    expect(generationError(new TypeError('Failed to fetch'))).toBe('Couldn’t generate a feed. Try again.');
});

test('a site that builds its posts in the browser says so and offers no retry', () => {
    const built = new ApiError(422, 'browser_built', "This site builds its post list in the browser, so Morgenblau can't follow it yet.");
    expect(generationError(built)).toBe('This site builds its post list in the browser, so Morgenblau can’t follow it yet.');
    expect(canRetryGeneration(built)).toBe(false);
    expect(generationError(new ApiError(502, '', "Couldn't read that page"))).toBe('Couldn’t read that page.');
});

test('only a passing fault offers to generate again', () => {
    expect(canRetryGeneration(new ApiError(502, 'upstream_error', 'x'))).toBe(true);
    expect(canRetryGeneration(new ApiError(500, 'internal_error', 'x'))).toBe(true);
    expect(canRetryGeneration(new ApiError(504, undefined, 'x'))).toBe(true);
    expect(canRetryGeneration(new TypeError('Failed to fetch'))).toBe(true);
    expect(canRetryGeneration(new ApiError(429, 'rate_limited', 'x'))).toBe(false);
    expect(canRetryGeneration(new ApiError(400, 'invalid_request', 'x'))).toBe(false);
    expect(canRetryGeneration(new ApiError(503, 'unavailable', 'x'))).toBe(false);
    expect(canRetryGeneration(new Error('Enter a website URL.'))).toBe(false);
});

test('a generated feed is offered by its health', () => {
    expect(generatedVerdict(generated)).toEqual({ subscribable: true, askAnchor: false, blocked: false });
    expect(generatedVerdict({ ...generated, needsAnchor: true })).toEqual({ subscribable: true, askAnchor: true, blocked: false });
    expect(generatedVerdict({ ...generated, health: 'needs_attention' })).toEqual({ subscribable: true, askAnchor: true, blocked: false });
    expect(generatedVerdict({ ...generated, health: 'blocked_by_site', needsAnchor: true })).toEqual({ subscribable: false, askAnchor: false, blocked: true });
    expect(generatedVerdict({ ...generated, health: 'broken', items: [], itemCount: 0 })).toEqual({ subscribable: false, askAnchor: true, blocked: false });
    expect(generatedVerdict({ ...generated, items: [], itemCount: 0 })).toEqual({ subscribable: false, askAnchor: true, blocked: false });
});

test('a shared feed that failed its last poll still offers the posts it serves', () => {
    expect(generatedVerdict({ ...generated, health: 'broken' })).toEqual({ subscribable: true, askAnchor: true, blocked: false });
});

async function limitedBy(code: string, retryAfter: string | null) {
    respond(() =>
        Response.json(
            { code, message: 'Server copy' },
            { status: 429, headers: retryAfter === null ? {} : { 'Retry-After': retryAfter } },
        ),
    );
    return generateSource('https://news.example.com/', undefined, new AbortController().signal).catch(
        (error: unknown) => error,
    );
}

test('a setup still in flight reads as a short wait that can be tried again', async () => {
    const inFlight = await limitedBy('setup_in_flight', '10');
    expect(generationError(inFlight)).toBe('Your last feed is still being generated. Try again in a moment.');
    expect(canRetryGeneration(inFlight)).toBe(true);
    expect(anchorError(inFlight)).toEqual({
        message: 'Your last feed is still being generated. Try again in a moment.',
        invalid: false,
    });
});

test('a long limit reads calmly, names no window, and is not offered again at once', async () => {
    // A fixed hourly window can be seconds from its reset, which must not read as a setup in flight.
    for (const retryAfter of ['5', '600', '82800', null]) {
        const limited = await limitedBy('rate_limited', retryAfter);
        expect(generationError(limited)).toBe('You’ve generated a lot of feeds recently. Try again later.');
        expect(canRetryGeneration(limited)).toBe(false);
        expect(anchorError(limited)).toEqual({
            message: 'You’ve generated a lot of feeds recently. Try again later.',
            invalid: false,
        });
    }
});

test('a generated feed subscribes through the ordinary add path with its title and site', async () => {
    const bodies: unknown[] = [];
    respond((url, init) => {
        expect(url).toBe('/api/subscriptions');
        bodies.push(JSON.parse(String(init?.body)));
        return Response.json({ rkey: '3example' });
    });
    await addSource(generatedCandidate(generated));
    expect(bodies).toEqual([
        { feedUrl: 'https://app.example.com/feed/abc', title: 'Example Publication', siteUrl: 'https://news.example.com/' },
    ]);
});

test('generation takes the page as the reader typed it', async () => {
    respond((_url, init) => {
        expect(JSON.parse(String(init?.body))).toEqual({ url: 'https://news.example.com/blog' });
        return Response.json(generated);
    });
    await generateSource('  news.example.com/blog ', undefined, new AbortController().signal);
});

test('only a bad post address is the reader’s to fix', () => {
    expect(anchorError(new Error('Enter the address of one post on that page.'))).toEqual({
        message: 'Enter the address of one post on that page.',
        invalid: true,
    });
    expect(anchorError(new ApiError(400, 'invalid_request', 'Enter a web address'))).toEqual({
        message: 'Enter the address of one post on that page.',
        invalid: true,
    });
    expect(anchorError(new ApiError(503, 'unavailable', "Feed generation isn't available yet"))).toEqual({
        message: 'Feed generation isn’t available right now.',
        invalid: false,
    });
    expect(anchorError(new TypeError('Failed to fetch')).invalid).toBe(false);
});

test('a post that left the feed as it was says so calmly, by cause', () => {
    expect(anchorRejection('not_on_page')).toBe(
        'That post isn’t linked on this page. The feed stays as it was.',
    );
    expect(anchorRejection('not_confirmed')).toBe(
        'That post didn’t help. The feed stays as it was.',
    );
    expect(anchorRejection('no_better')).toBe(
        'That post didn’t help. The feed stays as it was.',
    );
});
