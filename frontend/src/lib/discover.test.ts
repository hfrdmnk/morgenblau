import { describe, expect, test } from 'bun:test';

import { ApiError } from '@/lib/api';
import { truncateDid } from '@/lib/handle';
import type { Profile } from '@/lib/profile';
import {
    classifySubscribeError,
    discoverFollowerLabel,
    discoverHidePayload,
    discoverReasonDids,
    discoverSourceFaviconUrl,
    discoverSourceLinkHref,
    discoverSourceTitle,
    discoverSubscribeOverridePayload,
    discoverSubscribePayload,
    formatDiscoverReason,
    pickSubscribeTopLevelError,
    resolveDiscoverReasonDisplay,
    type DiscoverReason,
    type DiscoverSource,
    type DiscoverSourceCard,
} from './discover';

describe('formatDiscoverReason', () => {
    test('names the top follower when exactly one strong-tier person subscribes', () => {
        expect(
            formatDiscoverReason(
                { strongCount: 1, weakCount: 0, topFollowerDid: 'did:plc:alice' },
                '@alice',
            ),
        ).toBe('@alice subscribes');
    });

    test('falls back to a count phrase when no label is available for a single strong follower', () => {
        expect(
            formatDiscoverReason({
                strongCount: 1,
                weakCount: 0,
                topFollowerDid: 'did:plc:alice',
            }),
        ).toBe('1 person you follow subscribes');
    });

    test('uses a count phrase for multiple followers', () => {
        expect(
            formatDiscoverReason({ strongCount: 3, weakCount: 0 }, '@alice'),
        ).toBe('3 people you follow subscribe');
    });

    test('sums strong and weak counts for the multi-follower phrase', () => {
        expect(formatDiscoverReason({ strongCount: 2, weakCount: 3 })).toBe(
            '5 people you follow subscribe',
        );
    });

    test('multi-follower phrase uses the top signal verb, not a hardcoded subscribe', () => {
        expect(
            formatDiscoverReason({ strongCount: 2, weakCount: 0, topSignal: 'share' }),
        ).toBe('2 people you follow shared this');
        expect(
            formatDiscoverReason({ strongCount: 3, weakCount: 0, topSignal: 'author' }),
        ).toBe('3 people you follow write this');
        expect(
            formatDiscoverReason({ strongCount: 2, weakCount: 0, topSignal: 'subscribe' }),
        ).toBe('2 people you follow subscribe');
    });

    // Backend pre-filters counts to the top signal, so this formatter never sees a mixed group to collapse.
    test('a count arriving pre-filtered to the top signal reads as that many contributors, not the whole group', () => {
        expect(
            formatDiscoverReason({ strongCount: 1, weakCount: 0, topSignal: 'author' }),
        ).toBe('1 person you follow writes this');
    });

    test('names the network, not a label, for a sole Bluesky follower', () => {
        expect(
            formatDiscoverReason(
                {
                    strongCount: 0,
                    weakCount: 1,
                    topFollowerDid: 'did:plc:bsky-alice',
                    topFollowerNetwork: 'bluesky',
                },
                '@should-be-ignored',
            ),
        ).toBe('you follow on Bluesky');
    });

    test('names the network for a sole Tangled follower', () => {
        expect(
            formatDiscoverReason({
                strongCount: 0,
                weakCount: 1,
                topFollowerDid: 'did:plc:tangled-dev',
                topFollowerNetwork: 'tangled',
            }),
        ).toBe('you follow on Tangled');
    });

    test('a labeled author signal reads "writes this"', () => {
        expect(
            formatDiscoverReason(
                {
                    strongCount: 1,
                    weakCount: 0,
                    topFollowerDid: 'did:plc:bob',
                    topSignal: 'author',
                },
                '@bob',
            ),
        ).toBe('@bob writes this');
    });

    test('a labeled share signal reads "shared this"', () => {
        expect(
            formatDiscoverReason(
                {
                    strongCount: 1,
                    weakCount: 0,
                    topFollowerDid: 'did:plc:alice',
                    topSignal: 'share',
                },
                '@alice',
            ),
        ).toBe('@alice shared this');
    });

    test('an unlabeled author signal falls back to a count phrase with the author verb', () => {
        expect(
            formatDiscoverReason({
                strongCount: 1,
                weakCount: 0,
                topFollowerDid: 'did:plc:bob',
                topSignal: 'author',
            }),
        ).toBe('1 person you follow writes this');
    });

    test('a missing topSignal defaults to the subscribe verb (older responses)', () => {
        expect(
            formatDiscoverReason(
                { strongCount: 1, weakCount: 0, topFollowerDid: 'did:plc:alice' },
                '@alice',
            ),
        ).toBe('@alice subscribes');
    });

    test('an unrecognized topSignal (a cached "save" from before the purge) falls back to subscribe', () => {
        expect(
            formatDiscoverReason(
                {
                    strongCount: 1,
                    weakCount: 0,
                    topFollowerDid: 'did:plc:alice',
                    topSignal: 'save' as DiscoverReason['topSignal'],
                },
                '@alice',
            ),
        ).toBe('@alice subscribes');
        expect(
            formatDiscoverReason({
                strongCount: 2,
                weakCount: 0,
                topSignal: 'save' as DiscoverReason['topSignal'],
            }),
        ).toBe('2 people you follow subscribe');
    });

    test('a self-tier Skyreader subscription reads "you subscribe on Skyreader"', () => {
        expect(
            formatDiscoverReason({
                strongCount: 0,
                weakCount: 0,
                selfSourceApp: 'skyreader',
            }),
        ).toBe('you subscribe on Skyreader');
    });

    test('a self-tier Glean subscription reads "you subscribe on Glean"', () => {
        expect(
            formatDiscoverReason({
                strongCount: 0,
                weakCount: 0,
                selfSourceApp: 'glean',
            }),
        ).toBe('you subscribe on Glean');
    });

    test('a self-tier reason takes priority over strong/weak counts and labels', () => {
        expect(
            formatDiscoverReason(
                {
                    strongCount: 3,
                    weakCount: 1,
                    topFollowerDid: 'did:plc:alice',
                    topFollowerNetwork: 'bluesky',
                    selfSourceApp: 'skyreader',
                },
                '@alice',
            ),
        ).toBe('you subscribe on Skyreader');
    });

});

describe('resolveDiscoverReasonDisplay', () => {
    test('self-source wins even alongside strong-tier counts (regression guard)', () => {
        const reason: DiscoverReason = {
            strongCount: 3,
            weakCount: 0,
            selfSourceApp: 'skyreader',
        };
        expect(resolveDiscoverReasonDisplay(reason)).toEqual({
            kind: 'text',
            text: 'you subscribe on Skyreader',
        });
    });

    test('a sole author signal resolves to the author kind', () => {
        const reason: DiscoverReason = {
            strongCount: 1,
            weakCount: 0,
            topSignal: 'author',
            authorDid: 'did:plc:author',
        };
        expect(resolveDiscoverReasonDisplay(reason)).toEqual({
            kind: 'author',
            did: 'did:plc:author',
        });
    });

    test('an author signal shared by two people resolves to people, not author', () => {
        const reason: DiscoverReason = {
            strongCount: 2,
            weakCount: 0,
            topSignal: 'author',
            authorDid: 'did:plc:author',
        };
        expect(resolveDiscoverReasonDisplay(reason)).toEqual({
            kind: 'people',
            followerDids: [],
            total: 2,
        });
    });

    test('any positive follower count resolves to people, capped at 3 avatars', () => {
        const reason: DiscoverReason = {
            strongCount: 3,
            weakCount: 2,
            followerDids: [
                'did:plc:one',
                'did:plc:two',
                'did:plc:three',
                'did:plc:four',
            ],
        };
        expect(resolveDiscoverReasonDisplay(reason)).toEqual({
            kind: 'people',
            followerDids: ['did:plc:one', 'did:plc:two', 'did:plc:three'],
            total: 5,
        });
    });

    test('personal signal suppresses trending', () => {
        const reason: DiscoverReason = {
            strongCount: 1,
            weakCount: 0,
            trending: true,
        };
        expect(resolveDiscoverReasonDisplay(reason)).toEqual({
            kind: 'people',
            followerDids: [],
            total: 1,
        });
    });

    test('trending-only resolves to the trending kind', () => {
        const reason: DiscoverReason = {
            strongCount: 0,
            weakCount: 0,
            trending: true,
        };
        expect(resolveDiscoverReasonDisplay(reason)).toEqual({
            kind: 'trending',
        });
    });

    test('nothing at all falls back to "New to your network"', () => {
        const reason: DiscoverReason = { strongCount: 0, weakCount: 0 };
        expect(resolveDiscoverReasonDisplay(reason)).toEqual({
            kind: 'text',
            text: 'New to your network',
        });
    });
});

describe('discoverReasonDids', () => {
    test('dedups across authorDid, topFollowerDid, and followerDids', () => {
        const reason: DiscoverReason = {
            strongCount: 2,
            weakCount: 0,
            authorDid: 'did:plc:alice',
            topFollowerDid: 'did:plc:alice',
            followerDids: ['did:plc:alice', 'did:plc:bob'],
        };
        expect(discoverReasonDids(reason)).toEqual([
            'did:plc:alice',
            'did:plc:bob',
        ]);
    });

    test('returns an empty list when no DIDs are present', () => {
        expect(discoverReasonDids({ strongCount: 0, weakCount: 0 })).toEqual(
            [],
        );
    });
});

describe('discoverFollowerLabel', () => {
    test('prefers the resolved handle', () => {
        const profile: Profile = {
            did: 'did:plc:alice',
            handle: 'alice.example',
        };
        expect(discoverFollowerLabel('did:plc:alice', profile)).toBe(
            '@alice.example',
        );
    });

    test('falls back to a truncated DID when no profile resolved', () => {
        const did = 'did:plc:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa';
        expect(discoverFollowerLabel(did, undefined)).toBe(truncateDid(did));
    });
});

describe('discoverSubscribePayload', () => {
    test('rss: carries feedUrl, title, and siteUrl', () => {
        const source: DiscoverSource = {
            key: 'https://example.com/feed',
            kind: 'rss',
            title: 'Example',
            siteUrl: 'https://example.com',
            feedUrl: 'https://example.com/feed',
            reason: { strongCount: 2, weakCount: 0 },
        };
        expect(discoverSubscribePayload(source)).toEqual({
            feedUrl: 'https://example.com/feed',
            title: 'Example',
            siteUrl: 'https://example.com',
            primary: false,
            tags: [],
        });
    });

    test('rss: missing title/siteUrl fall back to empty strings', () => {
        const source: DiscoverSource = {
            key: 'https://example.com/feed',
            kind: 'rss',
            feedUrl: 'https://example.com/feed',
            reason: { strongCount: 1, weakCount: 0 },
        };
        expect(discoverSubscribePayload(source)).toEqual({
            feedUrl: 'https://example.com/feed',
            title: '',
            siteUrl: '',
            primary: false,
            tags: [],
        });
    });

    test('standardfeed: sends a blank title so no sidecar record is created', () => {
        const source: DiscoverSource = {
            key: 'at://did:plc:pub/site.standard.publication/3p',
            kind: 'standardfeed',
            title: 'Zine',
            publication: 'at://did:plc:pub/site.standard.publication/3p',
            reason: { strongCount: 1, weakCount: 0 },
        };
        expect(discoverSubscribePayload(source)).toEqual({
            publication: 'at://did:plc:pub/site.standard.publication/3p',
            title: '',
            primary: false,
            tags: [],
        });
    });
});

describe('discoverSubscribeOverridePayload', () => {
    test('rss: keeps feedUrl/siteUrl, applies the form title/primary/tags', () => {
        const source: DiscoverSource = {
            key: 'https://example.com/feed',
            kind: 'rss',
            title: 'Example',
            siteUrl: 'https://example.com',
            feedUrl: 'https://example.com/feed',
            reason: { strongCount: 1, weakCount: 0 },
        };
        expect(
            discoverSubscribeOverridePayload(source, {
                title: 'My title',
                primary: true,
                tags: ['news'],
            }),
        ).toEqual({
            feedUrl: 'https://example.com/feed',
            siteUrl: 'https://example.com',
            title: 'My title',
            primary: true,
            tags: ['news'],
        });
    });

    test('standardfeed: sends a blank title when the form title matches the prefill', () => {
        const source: DiscoverSource = {
            key: 'at://did:plc:pub/site.standard.publication/3p',
            kind: 'standardfeed',
            title: 'Zine',
            publication: 'at://did:plc:pub/site.standard.publication/3p',
            reason: { strongCount: 1, weakCount: 0 },
        };
        expect(
            discoverSubscribeOverridePayload(source, {
                title: 'Zine',
                primary: false,
                tags: [],
            }),
        ).toEqual({
            publication: 'at://did:plc:pub/site.standard.publication/3p',
            title: '',
            primary: false,
            tags: [],
        });
    });

    test('standardfeed: keeps a title the user changed from the prefill', () => {
        const source: DiscoverSource = {
            key: 'at://did:plc:pub/site.standard.publication/3p',
            kind: 'standardfeed',
            title: 'Zine',
            publication: 'at://did:plc:pub/site.standard.publication/3p',
            reason: { strongCount: 1, weakCount: 0 },
        };
        expect(
            discoverSubscribeOverridePayload(source, {
                title: 'My Zine',
                primary: false,
                tags: [],
            }),
        ).toEqual({
            publication: 'at://did:plc:pub/site.standard.publication/3p',
            title: 'My Zine',
            primary: false,
            tags: [],
        });
    });
});

describe('classifySubscribeError', () => {
    test('reauth: 403 + reauth_required maps to the reauth kind', () => {
        const error = new ApiError(403, 'reauth_required', 'nope');
        expect(classifySubscribeError(error)).toEqual({ kind: 'reauth' });
    });

    test('field errors map to the fields kind', () => {
        const errors = { 'subscriptions.0.title': 'Title is required' };
        const error = new ApiError(422, 'invalid', 'nope', errors);
        expect(classifySubscribeError(error)).toEqual({
            kind: 'fields',
            errors,
        });
    });

    test('a plain ApiError maps to a message using the server text', () => {
        const error = new ApiError(500, undefined, 'Server exploded');
        expect(classifySubscribeError(error)).toEqual({
            kind: 'message',
            message: 'Server exploded',
        });
    });

    test('a non-ApiError falls back to the caller-supplied message', () => {
        expect(
            classifySubscribeError(new Error('network down'), 'Try again?'),
        ).toEqual({ kind: 'message', message: 'Try again?' });
    });
});

describe('pickSubscribeTopLevelError', () => {
    test('prefers a feedUrl field error', () => {
        expect(
            pickSubscribeTopLevelError(
                { 'subscriptions.0.feedUrl': 'bad url' },
                'fallback',
            ),
        ).toBe('bad url');
    });

    test('falls back to a publication field error', () => {
        expect(
            pickSubscribeTopLevelError(
                { 'subscriptions.0.publication': 'bad publication' },
                'fallback',
            ),
        ).toBe('bad publication');
    });

    test('falls back to the general subscriptions error', () => {
        expect(
            pickSubscribeTopLevelError(
                { subscriptions: 'general error' },
                'fallback',
            ),
        ).toBe('general error');
    });

    test('falls back to the submit error when no field errors are set', () => {
        expect(pickSubscribeTopLevelError({}, 'fallback')).toBe('fallback');
    });

    test('is undefined when nothing is set', () => {
        expect(pickSubscribeTopLevelError({}, undefined)).toBeUndefined();
    });
});

describe('discoverHidePayload', () => {
    test('builds a source-kind hide payload keyed by the canonical key', () => {
        const source: DiscoverSource = {
            key: 'https://example.com/feed',
            kind: 'rss',
            feedUrl: 'https://example.com/feed',
            reason: { strongCount: 1, weakCount: 0 },
        };
        expect(discoverHidePayload(source)).toEqual({
            targetKind: 'source',
            targetKey: 'https://example.com/feed',
        });
    });

    test('standardfeed: keys the hide by the publication at-uri (the canonical key)', () => {
        const source: DiscoverSource = {
            key: 'at://did:plc:pub/site.standard.publication/3p',
            kind: 'standardfeed',
            publication: 'at://did:plc:pub/site.standard.publication/3p',
            reason: { strongCount: 1, weakCount: 0 },
        };
        expect(discoverHidePayload(source)).toEqual({
            targetKind: 'source',
            targetKey: 'at://did:plc:pub/site.standard.publication/3p',
        });
    });
});

describe('discoverSourceTitle', () => {
    test('prefers title', () => {
        expect(
            discoverSourceTitle({
                key: 'k',
                kind: 'rss',
                title: 'Title',
                siteUrl: 'https://site.example',
                feedUrl: 'https://feed.example',
            }),
        ).toBe('Title');
    });

    test('falls back to the siteUrl hostname, with www stripped', () => {
        expect(
            discoverSourceTitle({
                key: 'k',
                kind: 'rss',
                siteUrl: 'https://www.site.example',
                feedUrl: 'https://feed.example',
            }),
        ).toBe('site.example');
    });

    test('falls back to the feedUrl hostname when siteUrl is absent', () => {
        expect(
            discoverSourceTitle({
                key: 'k',
                kind: 'rss',
                feedUrl: 'https://feed.example/rss.xml',
            }),
        ).toBe('feed.example');
    });

    test('an unparseable siteUrl falls through to the feedUrl hostname', () => {
        expect(
            discoverSourceTitle({
                key: 'k',
                kind: 'rss',
                siteUrl: 'not a url',
                feedUrl: 'https://feed.example',
            }),
        ).toBe('feed.example');
    });

    test('falls back to publication, then key, for non-URL sources', () => {
        expect(
            discoverSourceTitle({
                key: 'at://x',
                kind: 'standardfeed',
                publication: 'at://x',
            }),
        ).toBe('at://x');
        expect(
            discoverSourceTitle({
                key: 'bare-key',
                kind: 'rss',
            }),
        ).toBe('bare-key');
    });
});

describe('discoverSourceLinkHref', () => {
    test('siteUrl wins over feedUrl', () => {
        expect(
            discoverSourceLinkHref({
                key: 'k',
                kind: 'rss',
                siteUrl: 'https://site.example',
                feedUrl: 'https://feed.example/rss.xml',
            }),
        ).toBe('https://site.example');
    });

    test('falls back to the feed origin when siteUrl is absent', () => {
        expect(
            discoverSourceLinkHref({
                key: 'k',
                kind: 'rss',
                feedUrl: 'https://feed.example/path/rss.xml?x=1',
            }),
        ).toBe('https://feed.example');
    });

    test('undefined when neither siteUrl nor feedUrl are present', () => {
        expect(
            discoverSourceLinkHref({ key: 'k', kind: 'standardfeed' }),
        ).toBeUndefined();
    });

    test('undefined when feedUrl is unparseable and siteUrl is absent', () => {
        expect(
            discoverSourceLinkHref({
                key: 'k',
                kind: 'rss',
                feedUrl: 'not a url',
            }),
        ).toBeUndefined();
    });

    test('rejects an unsafe siteUrl scheme, falling through to the feed origin', () => {
        expect(
            discoverSourceLinkHref({
                key: 'k',
                kind: 'rss',
                siteUrl: 'javascript:alert(1)',
                feedUrl: 'https://feed.example',
            }),
        ).toBe('https://feed.example');
    });
});

describe('discoverSourceFaviconUrl', () => {
    test('keys by feedUrl when present, encoding it', () => {
        const source: DiscoverSourceCard = {
            key: 'https://example.com/feed',
            kind: 'rss',
            feedUrl: 'https://example.com/feed?a=b',
        };
        expect(discoverSourceFaviconUrl(source)).toBe(
            '/api/favicon?feed=https%3A%2F%2Fexample.com%2Ffeed%3Fa%3Db',
        );
    });

    test('falls back to key for a standardfeed, which has no feedUrl', () => {
        const source: DiscoverSourceCard = {
            key: 'at://did:plc:pub/site.standard.publication/3p',
            kind: 'standardfeed',
            publication: 'at://did:plc:pub/site.standard.publication/3p',
        };
        expect(discoverSourceFaviconUrl(source)).toBe(
            '/api/favicon?feed=at%3A%2F%2Fdid%3Aplc%3Apub%2Fsite.standard.publication%2F3p',
        );
    });
});
