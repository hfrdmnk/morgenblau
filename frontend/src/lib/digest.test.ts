import { describe, expect, test } from 'bun:test';

import { digestRequestPath } from './digest';

describe('digestRequestPath', () => {
    test('sends the selected browser-local day and IANA timezone', () => {
        const date = new Date(2026, 8, 20);

        expect(digestRequestPath(date, 'Europe/Zurich')).toBe(
            '/api/digest?date=2026-09-20&timezone=Europe%2FZurich',
        );
    });
});
