import { describe, expect, test } from 'bun:test';

import { classifyShareResponse } from './share-response';

describe('classifyShareResponse', () => {
    test('reauth for a 403 carrying reauth_required', () => {
        expect(classifyShareResponse(403, { code: 'reauth_required' })).toBe(
            'reauth',
        );
    });

    test('failed for a 403 with a different code', () => {
        expect(classifyShareResponse(403, { code: 'forbidden' })).toBe(
            'failed',
        );
    });

    test('failed for a 403 with no body', () => {
        expect(classifyShareResponse(403, null)).toBe('failed');
    });

    test('ok for 200', () => {
        expect(classifyShareResponse(200, null)).toBe('ok');
    });

    test('ok for 204 no-content (the DELETE success)', () => {
        expect(classifyShareResponse(204, null)).toBe('ok');
    });

    test('failed for a 502', () => {
        expect(classifyShareResponse(502, null)).toBe('failed');
    });

    test('reauth_required only counts on a 403, not a 500', () => {
        expect(classifyShareResponse(500, { code: 'reauth_required' })).toBe(
            'failed',
        );
    });
});
