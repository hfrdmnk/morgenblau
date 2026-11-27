import { expect, test } from 'bun:test';

import { visits } from './visits';

test('first file sees only its own visit', () => {
    visits.push('first');
    expect(visits).toEqual(['first']);
});
