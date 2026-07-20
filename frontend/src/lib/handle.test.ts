import { describe, expect, test } from 'bun:test';

import { personRowLines } from './handle';

const DID = 'did:plc:abc123def456ghi789jk';

describe('personRowLines', () => {
    test('display name leads, handle label is dropped by default', () => {
        expect(
            personRowLines({
                handle: 'alice.example',
                displayName: 'Alice Example',
                did: DID,
            }),
        ).toEqual({ primary: 'Alice Example', secondary: undefined });
    });

    test('falls back to the @handle when there is no display name', () => {
        expect(
            personRowLines({ handle: 'alice.example', displayName: undefined, did: DID }),
        ).toEqual({ primary: '@alice.example', secondary: undefined });
    });

    test('falls back to the truncated DID without a handle', () => {
        const lines = personRowLines({ handle: undefined, displayName: undefined, did: DID });
        expect(lines.primary).toBe(`${DID.slice(0, 12)}…${DID.slice(-4)}`);
        expect(lines.secondary).toBeUndefined();
    });

    test('whitespace-only display names count as absent', () => {
        expect(
            personRowLines({ handle: 'alice.example', displayName: '   ', did: DID }),
        ).toEqual({ primary: '@alice.example', secondary: undefined });
    });

    test('handleAsSecondary shows the label under a display name', () => {
        expect(
            personRowLines({
                handle: 'alice.example',
                displayName: 'Alice Example',
                did: DID,
                handleAsSecondary: true,
            }),
        ).toEqual({ primary: 'Alice Example', secondary: '@alice.example' });
    });

    test('handleAsSecondary stays empty when the label already leads', () => {
        expect(
            personRowLines({
                handle: 'alice.example',
                displayName: undefined,
                did: DID,
                handleAsSecondary: true,
            }),
        ).toEqual({ primary: '@alice.example', secondary: undefined });
    });

    test('an explicit secondary wins over handleAsSecondary', () => {
        expect(
            personRowLines({
                handle: 'alice.example',
                displayName: 'Alice Example',
                did: DID,
                secondary: 'Followed by Example Person',
                handleAsSecondary: true,
            }),
        ).toEqual({
            primary: 'Alice Example',
            secondary: 'Followed by Example Person',
        });
    });
});
