import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import path from 'node:path';

import { OFFLINE_IMAGES } from '@/lib/api';

test('the service worker stores only images, in the cache that clearOfflineCopies deletes', () => {
    const config = readFileSync(path.join(import.meta.dir, '..', 'vite.config.ts'), 'utf8');
    // Documents go network-only with the precached shell as fallback; the one storing route is the image cache.
    expect(config.match(/handler: '\w+'/g)).toEqual(["handler: 'NetworkOnly'", "handler: 'CacheFirst'"]);
    expect(config.match(/cacheName: '[\w-]+'/g)).toEqual([`cacheName: '${OFFLINE_IMAGES}'`]);
    // injectManifest hands routing to a custom service worker, which could register /api routes.
    expect(config).not.toContain('injectManifest');
});
