import { toastMutationError } from '@/lib/mutation-toast';

import type { DiscoverPage } from './discover-page';

type Options<TWire, TItem> = {
    cursor?: string;
    loading: boolean;
    items: TItem[];
    keyOfItem: (item: TItem) => string;
    keyOfWire: (item: TWire) => string;
    fetchPage: (cursor: string) => Promise<DiscoverPage<TWire>>;
    start: () => void;
    append: (page: DiscoverPage<TWire>) => void;
    fail: () => void;
    hydrate: (items: TWire[]) => Promise<void>;
    cancelled: () => boolean;
};

export function loadMoreDiscoverItems<TWire, TItem>(
    options: Options<TWire, TItem>,
): void {
    if (!options.cursor || options.loading) return;

    const existing = new Set(options.items.map(options.keyOfItem));
    options.start();
    void requestDiscoverPage(options, options.cursor, existing);
}

async function requestDiscoverPage<TWire, TItem>(
    options: Options<TWire, TItem>,
    cursor: string,
    existing: ReadonlySet<string>,
): Promise<void> {
    try {
        const page = await options.fetchPage(cursor);
        if (options.cancelled()) return;

        options.append(page);
        const additions = page.items.filter(
            (item) => !existing.has(options.keyOfWire(item)),
        );
        await options.hydrate(additions);
    } catch (error) {
        if (options.cancelled()) return;
        options.fail();
        toastMutationError(error, "Couldn't load more. Try again.");
    }
}
