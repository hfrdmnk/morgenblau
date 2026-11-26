import { afterEach } from 'bun:test';

// Motion binds its frame loop to the requestAnimationFrame it sees on first import, and every test file shares that
// import. A frame scheduled on a test file's happy-dom timers dies with that window and leaves motion waiting forever,
// so frames run on Bun's own timer, captured here before any file registers happy-dom.
const nativeSetTimeout = setTimeout;
globalThis.requestAnimationFrame = (callback) => Number(nativeSetTimeout(() => callback(performance.now()), 16));
await import('motion/react');

// Browsers give every secure page Cache Storage and happy-dom does not, so api()'s offline copies get an in-memory one.
const stores = new Map<string, Map<string, Response>>();
const cacheKey = (key: RequestInfo | URL) => {
    const url = new URL(key instanceof Request ? key.url : String(key), 'http://localhost');
    return url.pathname + url.search;
};
const memoryCaches: Pick<CacheStorage, 'open' | 'delete'> = {
    async open(name) {
        const entries = stores.get(name) ?? new Map<string, Response>();
        stores.set(name, entries);
        const cache: Pick<Cache, 'match' | 'put' | 'keys' | 'delete'> = {
            async match(key) {
                return entries.get(cacheKey(key))?.clone();
            },
            // Like Cache Storage, a rewritten entry moves to the end of the list.
            async put(key, response) {
                entries.delete(cacheKey(key));
                entries.set(cacheKey(key), response);
            },
            async keys() {
                return [...entries.keys()].map((key) => new Request(new URL(key, 'http://localhost')));
            },
            async delete(key) {
                return entries.delete(cacheKey(key));
            },
        };
        return cache as Cache;
    },
    async delete(name) {
        return stores.delete(name);
    },
};
globalThis.caches = memoryCaches as CacheStorage;
afterEach(() => stores.clear());
