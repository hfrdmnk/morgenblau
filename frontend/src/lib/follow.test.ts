import { describe, expect, test } from 'bun:test';

import { ApiError } from './api';
import {
    classifyFollowError,
    normalizeHandleInput,
    parseFollowResponse,
} from './follow';

describe('normalizeHandleInput', () => {
    test('trims surrounding whitespace', () => {
        expect(normalizeHandleInput('  alice.bsky.social  ')).toBe(
            'alice.bsky.social',
        );
    });

    test('strips a single leading @', () => {
        expect(normalizeHandleInput('@alice.bsky.social')).toBe(
            'alice.bsky.social',
        );
    });

    test('trims then strips @, in that order', () => {
        expect(normalizeHandleInput('  @alice.bsky.social  ')).toBe(
            'alice.bsky.social',
        );
    });

    test('leaves a bare handle untouched', () => {
        expect(normalizeHandleInput('alice.bsky.social')).toBe(
            'alice.bsky.social',
        );
    });

    test('empty input stays empty', () => {
        expect(normalizeHandleInput('   ')).toBe('');
    });
});

describe('classifyFollowError', () => {
    test('surfaces the backend message from an ApiError', () => {
        const error = new ApiError(422, undefined, "couldn't find that handle");
        expect(classifyFollowError(error)).toBe("couldn't find that handle");
    });

    test('falls back to a generic message for a non-ApiError failure', () => {
        expect(classifyFollowError(new TypeError('network error'))).toEqual(
            expect.any(String),
        );
    });

    test('falls back to a generic message when the error is not an Error at all', () => {
        expect(classifyFollowError('some rejection')).toEqual(
            expect.any(String),
        );
    });
});

describe('parseFollowResponse', () => {
    test('returns the record when both keys are present', () => {
        expect(
            parseFollowResponse({
                rkey: '3abc',
                subjectDid: 'did:plc:xyz',
                createdAt: '2026-07-11T00:00:00Z',
            }),
        ).toEqual({
            rkey: '3abc',
            subjectDid: 'did:plc:xyz',
            createdAt: '2026-07-11T00:00:00Z',
        });
    });

    test('defaults a missing createdAt to an empty string', () => {
        expect(
            parseFollowResponse({ rkey: '3abc', subjectDid: 'did:plc:xyz' }),
        ).toEqual({ rkey: '3abc', subjectDid: 'did:plc:xyz', createdAt: '' });
    });

    test('null for a missing rkey', () => {
        expect(parseFollowResponse({ subjectDid: 'did:plc:xyz' })).toBeNull();
    });

    test('null for a missing subjectDid', () => {
        expect(parseFollowResponse({ rkey: '3abc' })).toBeNull();
    });

    test('null for a null or undefined body', () => {
        expect(parseFollowResponse(null)).toBeNull();
        expect(parseFollowResponse(undefined)).toBeNull();
    });
});
