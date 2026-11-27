import { expect, test } from 'bun:test';
import { existsSync, readFileSync } from 'node:fs';
import path from 'node:path';

import { OFFLINE_IMAGES } from '@/lib/api';

const root = path.join(import.meta.dir, '..');
const config = readFileSync(path.join(root, 'vite.config.ts'), 'utf8');

test('the service worker stores only images, in the cache that clearOfflineCopies deletes', () => {
    expect(
        config.match(/handler: '\w+'/g),
        'Documents go network-only with the precached shell as fallback; the one storing route is the image cache.',
    ).toEqual(["handler: 'NetworkOnly'", "handler: 'CacheFirst'"]);
    expect(config.match(/cacheName: '[\w-]+'/g)).toEqual([`cacheName: '${OFFLINE_IMAGES}'`]);
    expect(
        config,
        'injectManifest hands routing to a custom service worker, which could register /api routes.',
    ).not.toContain('injectManifest');
});

test('documents go to the server, which redirects signed-out readers, and the precached shell answers only when the network fails', () => {
    expect(config).toContain('directoryIndex: null');
    expect(config).toContain('navigateFallback: null');
    expect(config).toContain("precacheFallback: { fallbackURL: '/index.html' }");
});

test('the precache holds webp, since the empty and error states paint their art as a CSS background an offline error screen needs', () => {
    expect(config).toMatch(/globPatterns: \['[^']*\bwebp\b[^']*'\]/);
});

test('every Apple screen has a startup image for each orientation, since iOS shows one only when its pixel size matches the screen exactly', () => {
    const screens = config.match(/const appleScreens[^=]*= \[([\s\S]*?)\n\];/)![1];
    const sizes = [...screens.matchAll(/\[(\d+), (\d+), (\d+)\]/g)].map(([, width, height, ratio]) => [
        Number(width) * Number(ratio),
        Number(height) * Number(ratio),
    ]);
    expect(sizes.length).toBeGreaterThan(0);
    const missing = sizes
        .flatMap(([width, height]) => [`${width}x${height}.png`, `${height}x${width}.png`])
        .filter((file) => !existsSync(path.join(root, 'public', 'splash', file)));
    expect(missing).toEqual([]);
});
