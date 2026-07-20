import { describe, expect, test } from 'bun:test';

import {
    formatPersonReason,
    peopleProfileDids,
    personHidePayload,
    personPreviewHiddenCount,
    personPreviewEmpty,
    personTastePreviewLabel,
    resolvePersonReasonDisplay,
    settlePeopleLoad,
    toFollowCard,
    toPersonCard,
    type DiscoverPerson,
    type PersonPreview,
    type PersonPreviewShare,
    type PersonPreviewSource,
    type PersonReason,
    type PersonTastePreview,
} from './discover-people';
import type { FollowRecord } from '@/lib/follow';

const NO_SIGNAL: PersonReason = {
    blueskyFollow: false,
    tangledFollow: false,
    sharedSourceCount: 0,
};

describe('formatPersonReason', () => {
    test('a Bluesky-only candidate reads "you follow on Bluesky"', () => {
        expect(
            formatPersonReason({ ...NO_SIGNAL, blueskyFollow: true }),
        ).toBe('you follow on Bluesky');
    });

    test('a Tangled-only candidate reads "you follow on Tangled"', () => {
        expect(
            formatPersonReason({ ...NO_SIGNAL, tangledFollow: true }),
        ).toBe('you follow on Tangled');
    });

    test('a one-hop candidate names the introducing friend when a label resolved', () => {
        const reason: PersonReason = {
            ...NO_SIGNAL,
            followedByDid: 'did:plc:alice',
        };
        expect(formatPersonReason(reason, '@alice')).toBe(
            'followed by @alice',
        );
    });

    test('a one-hop candidate falls back to a generic phrase without a resolved label', () => {
        const reason: PersonReason = {
            ...NO_SIGNAL,
            followedByDid: 'did:plc:alice',
        };
        expect(formatPersonReason(reason)).toBe(
            'followed by someone you follow',
        );
    });

    test('taste overlap outranks every other candidate-class phrasing', () => {
        const reason: PersonReason = {
            blueskyFollow: true,
            tangledFollow: true,
            followedByDid: 'did:plc:alice',
            sharedSourceCount: 4,
        };
        expect(formatPersonReason(reason, '@alice')).toBe(
            'reads 4 of your sources',
        );
    });

    test('followed-by outranks Bluesky and Tangled', () => {
        const reason: PersonReason = {
            blueskyFollow: true,
            tangledFollow: true,
            followedByDid: 'did:plc:alice',
            sharedSourceCount: 0,
        };
        expect(formatPersonReason(reason, '@alice')).toBe(
            'followed by @alice',
        );
    });

    test('a single shared source still uses the "reads N" phrasing', () => {
        expect(
            formatPersonReason({ ...NO_SIGNAL, sharedSourceCount: 1 }),
        ).toBe('reads 1 of your sources');
    });

    test('no signal at all falls back to "active in the reader network"', () => {
        expect(formatPersonReason(NO_SIGNAL)).toBe(
            'active in the reader network',
        );
    });
});

describe('resolvePersonReasonDisplay', () => {
    test('a shared-source signal resolves to text, trending or not', () => {
        const reason: PersonReason = {
            ...NO_SIGNAL,
            sharedSourceCount: 2,
            trending: true,
        };
        expect(resolvePersonReasonDisplay(reason)).toEqual({
            kind: 'text',
            text: 'reads 2 of your sources',
        });
    });

    test('a Bluesky-follow signal resolves to text', () => {
        const reason: PersonReason = { ...NO_SIGNAL, blueskyFollow: true };
        expect(resolvePersonReasonDisplay(reason)).toEqual({
            kind: 'text',
            text: 'you follow on Bluesky',
        });
    });

    test('a Tangled-follow signal resolves to text', () => {
        const reason: PersonReason = { ...NO_SIGNAL, tangledFollow: true };
        expect(resolvePersonReasonDisplay(reason)).toEqual({
            kind: 'text',
            text: 'you follow on Tangled',
        });
    });

    test('a sole followed-by signal resolves to the followedBy kind with the raw did, no pre-baked label', () => {
        const reason: PersonReason = {
            ...NO_SIGNAL,
            followedByDid: 'did:plc:alice',
        };
        expect(resolvePersonReasonDisplay(reason)).toEqual({
            kind: 'followedBy',
            did: 'did:plc:alice',
        });
    });

    test('followed-by plus trending still resolves to followedBy (personal suppresses trending)', () => {
        const reason: PersonReason = {
            ...NO_SIGNAL,
            followedByDid: 'did:plc:alice',
            trending: true,
        };
        expect(resolvePersonReasonDisplay(reason)).toEqual({
            kind: 'followedBy',
            did: 'did:plc:alice',
        });
    });

    test('trending-only resolves to the trending kind', () => {
        const reason: PersonReason = { ...NO_SIGNAL, trending: true };
        expect(resolvePersonReasonDisplay(reason)).toEqual({
            kind: 'trending',
        });
    });

    test('any personal signal suppresses the trending line (SPEC <discovery> line 576)', () => {
        const reason: PersonReason = {
            blueskyFollow: true,
            tangledFollow: false,
            sharedSourceCount: 0,
            trending: true,
        };
        expect(resolvePersonReasonDisplay(reason)).toEqual({
            kind: 'text',
            text: 'you follow on Bluesky',
        });
    });

    test('nothing at all falls back to the eligible-but-unranked text', () => {
        expect(resolvePersonReasonDisplay(NO_SIGNAL)).toEqual({
            kind: 'text',
            text: 'active in the reader network',
        });
    });
});

describe('toPersonCard', () => {
    test('adds a key mapped from the did', () => {
        const person: DiscoverPerson = {
            did: 'did:plc:alice',
            reason: NO_SIGNAL,
        };
        expect(toPersonCard(person)).toEqual({
            did: 'did:plc:alice',
            reason: NO_SIGNAL,
            key: 'did:plc:alice',
        });
    });

    test('carries the taste preview through untouched', () => {
        const tastePreview: PersonTastePreview = { titles: ['Example Zine'] };
        const person: DiscoverPerson = {
            did: 'did:plc:bob',
            reason: NO_SIGNAL,
            tastePreview,
        };
        expect(toPersonCard(person).tastePreview).toBe(tastePreview);
    });
});

describe('personTastePreviewLabel', () => {
    test('undefined preview yields no label', () => {
        expect(personTastePreviewLabel(undefined)).toBeUndefined();
    });

    test('lists a few source titles', () => {
        const preview: PersonTastePreview = {
            titles: ['Example Weekly', 'Example Digest'],
        };
        expect(personTastePreviewLabel(preview)).toBe(
            'Reads Example Weekly, Example Digest',
        );
    });

    test('falls back to the latest share comment when there are no titles', () => {
        const preview: PersonTastePreview = {
            latestShareComment: 'great read',
        };
        expect(personTastePreviewLabel(preview)).toBe(
            'Shared “great read”',
        );
    });

    test('falls back to the latest share URL hostname when there is no comment', () => {
        const preview: PersonTastePreview = {
            latestShareItemUrl: 'https://blog.example/some-post',
        };
        expect(personTastePreviewLabel(preview)).toBe('Shared blog.example');
    });

    test('an unparseable share URL uses generic copy', () => {
        const preview: PersonTastePreview = {
            latestShareItemUrl: 'not a url',
        };
        expect(personTastePreviewLabel(preview)).toBe('Shared item');
    });

    test('an empty preview yields no label', () => {
        expect(personTastePreviewLabel({})).toBeUndefined();
    });
});

describe('personHidePayload', () => {
    test('builds a person-kind hide payload keyed by DID', () => {
        expect(personHidePayload('did:plc:alice')).toEqual({
            targetKind: 'person',
            targetKey: 'did:plc:alice',
        });
    });
});

const SOURCE: PersonPreviewSource = {
    key: 'example.com/feed.xml',
    kind: 'rss',
    title: 'Example Publication',
    siteUrl: 'https://example.com',
    subscribed: false,
};

const SHARE: PersonPreviewShare = {
    itemUrl: 'https://example.com/post',
    createdAt: '2026-07-01T00:00:00Z',
};

describe('personPreviewEmpty', () => {
    test('true when writes, reads, and the latest share are all empty', () => {
        const preview: PersonPreview = { writes: [], reads: [], latestShare: null };
        expect(personPreviewEmpty(preview)).toBe(true);
    });

    test('false when writes has content', () => {
        const preview: PersonPreview = { writes: [SOURCE], reads: [], latestShare: null };
        expect(personPreviewEmpty(preview)).toBe(false);
    });

    test('false when reads has content', () => {
        const preview: PersonPreview = { writes: [], reads: [SOURCE], latestShare: null };
        expect(personPreviewEmpty(preview)).toBe(false);
    });

    test('false when a latest share is present', () => {
        const preview: PersonPreview = { writes: [], reads: [], latestShare: SHARE };
        expect(personPreviewEmpty(preview)).toBe(false);
    });
});

describe('personPreviewHiddenCount', () => {
    test('reports only records hidden by the preview cap', () => {
        expect(personPreviewHiddenCount(5, 2)).toBe(3);
        expect(personPreviewHiddenCount(2, 2)).toBe(0);
        expect(personPreviewHiddenCount(undefined, 2)).toBe(0);
    });
});

describe('peopleProfileDids', () => {
    test('unions person dids, followed-by dids, and follow subjects, in that order', () => {
        const people: DiscoverPerson[] = [
            {
                did: 'did:plc:aaa111',
                reason: { ...NO_SIGNAL, followedByDid: 'did:plc:bbb222' },
            },
            { did: 'did:plc:ccc333', reason: NO_SIGNAL },
        ];
        const follows: FollowRecord[] = [
            {
                rkey: '3jzcut4wsuw2n',
                subjectDid: 'did:plc:ddd444',
                createdAt: '2026-07-01T00:00:00Z',
            },
        ];
        expect(peopleProfileDids(people, follows)).toEqual([
            'did:plc:aaa111',
            'did:plc:ccc333',
            'did:plc:bbb222',
            'did:plc:ddd444',
        ]);
    });

    test('skips people with no followedByDid rather than pushing undefined', () => {
        const people: DiscoverPerson[] = [
            { did: 'did:plc:aaa111', reason: NO_SIGNAL },
        ];
        expect(peopleProfileDids(people, [])).toEqual(['did:plc:aaa111']);
    });

    test('may contain duplicates, since dedup happens elsewhere', () => {
        const people: DiscoverPerson[] = [
            { did: 'did:plc:aaa111', reason: NO_SIGNAL },
        ];
        const follows: FollowRecord[] = [
            {
                rkey: '3jzcut4wsuw2n',
                subjectDid: 'did:plc:aaa111',
                createdAt: '2026-07-01T00:00:00Z',
            },
        ];
        expect(peopleProfileDids(people, follows)).toEqual([
            'did:plc:aaa111',
            'did:plc:aaa111',
        ]);
    });
});

describe('toFollowCard', () => {
    test('copies the follow record and sets key to the rkey', () => {
        const follow: FollowRecord = {
            rkey: '3jzcut4wsuw2n',
            subjectDid: 'did:plc:aaa111',
            createdAt: '2026-07-01T00:00:00Z',
        };
        expect(toFollowCard(follow)).toEqual({
            ...follow,
            key: '3jzcut4wsuw2n',
        });
    });
});

describe('settlePeopleLoad', () => {
    const PERSON: DiscoverPerson = { did: 'did:plc:aaa111', reason: NO_SIGNAL };
    const FOLLOW: FollowRecord = {
        rkey: '3jzcut4wsuw2n',
        subjectDid: 'did:plc:bbb222',
        createdAt: '2026-07-01T00:00:00Z',
    };

    test('both settling fulfilled maps to ok states and marks the load complete', () => {
        const result = settlePeopleLoad(
            {
                status: 'fulfilled',
                value: { items: [PERSON], nextCursor: 'cursor-1' },
            },
            { status: 'fulfilled', value: [FOLLOW] },
        );
        expect(result).toEqual({
            suggest: {
                kind: 'ok',
                people: [toPersonCard(PERSON)],
                nextCursor: 'cursor-1',
                loadingMore: false,
            },
            follow: { kind: 'ok', follows: [toFollowCard(FOLLOW)] },
            people: [PERSON],
            nextCursor: 'cursor-1',
            follows: [FOLLOW],
            complete: true,
        });
    });

    test('a rejected people promise degrades suggest to error and leaves people empty', () => {
        const result = settlePeopleLoad(
            { status: 'rejected', reason: new Error('boom') },
            { status: 'fulfilled', value: [FOLLOW] },
        );
        expect(result.suggest).toEqual({ kind: 'error' });
        expect(result.people).toEqual([]);
        expect(result.follow).toEqual({ kind: 'ok', follows: [toFollowCard(FOLLOW)] });
        expect(result.complete).toBe(false);
    });

    test('a rejected follows promise degrades follow to error and leaves follows empty', () => {
        const result = settlePeopleLoad(
            { status: 'fulfilled', value: { items: [PERSON] } },
            { status: 'rejected', reason: new Error('boom') },
        );
        expect(result.follow).toEqual({ kind: 'error' });
        expect(result.follows).toEqual([]);
        expect(result.suggest).toEqual({
            kind: 'ok',
            people: [toPersonCard(PERSON)],
            nextCursor: undefined,
            loadingMore: false,
        });
        expect(result.complete).toBe(false);
    });

    test('fulfilled with null coerces to empty arrays and still counts as complete', () => {
        const result = settlePeopleLoad(
            { status: 'fulfilled', value: null },
            { status: 'fulfilled', value: null },
        );
        expect(result).toEqual({
            suggest: {
                kind: 'ok',
                people: [],
                nextCursor: undefined,
                loadingMore: false,
            },
            follow: { kind: 'ok', follows: [] },
            people: [],
            nextCursor: undefined,
            follows: [],
            complete: true,
        });
    });
});
