import { describe, expect, test } from 'bun:test';

import type { Entry } from '@/components/digest-rows';
import { entryActivation } from './entry-nav';

describe('entryActivation', () => {
    test('opens newsletter issues in the private reader', () => {
        const entry: Entry = {
            id: 'message-one',
            entrySlug: 'newsletter-one',
            title: 'A private issue',
            contentType: 'newsletter',
            publishedAt: '2026-07-01T00:00:00Z',
            source: {
                kind: 'newsletter',
                id: 'source-one',
                title: 'Example Newsletter',
                siteUrl: null,
                faviconUrl: null,
            },
            body: '<p>Private</p>',
        };

        expect(
            entryActivation(entry, { newsletterSourceId: 'source-one' }),
        ).toEqual({
            href: '/entry/newsletter-one?fromNewsletter=source-one',
            external: false,
        });
    });
});
