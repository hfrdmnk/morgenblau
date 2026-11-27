import { expect, test } from 'bun:test';

import { visits } from './visits';

test('second file sees only its own visit', () => {
    visits.push('second');
    expect(visits).toEqual(['second']);
});
