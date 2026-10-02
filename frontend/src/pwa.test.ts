import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import path from 'node:path';

test('the service worker never runtime-caches, so no /api response outlives the session that fetched it', () => {
    const config = readFileSync(path.join(import.meta.dir, '..', 'vite.config.ts'), 'utf8');
    expect(config).not.toContain('runtimeCaching');
    // injectManifest hands routing to a custom service worker, which could register /api routes.
    expect(config).not.toContain('injectManifest');
});
