import { afterEach } from 'bun:test';

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
            // Cache Storage removes a request's old entry and appends the new one:
            // https://w3c.github.io/ServiceWorker/#batch-cache-operations-algorithm
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
