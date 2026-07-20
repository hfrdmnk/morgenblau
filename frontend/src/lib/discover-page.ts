export type DiscoverPage<T> = {
    items: T[];
    nextCursor?: string;
};

export type DiscoverPageState<T extends { key: string }> = {
    items: T[];
    nextCursor?: string;
    loadingMore: boolean;
};

export type DiscoverPageAction<T extends { key: string }> =
    | { type: 'loadMore' }
    | { type: 'append'; page: DiscoverPage<T> }
    | { type: 'failed' };

export function reduceDiscoverPage<T extends { key: string }>(
    state: DiscoverPageState<T>,
    action: DiscoverPageAction<T>,
): DiscoverPageState<T> {
    if (action.type === 'loadMore') {
        return { ...state, loadingMore: true };
    }
    if (action.type === 'failed') {
        return { ...state, loadingMore: false };
    }

    const seen = new Set(state.items.map((item) => item.key));
    const additions = action.page.items.filter((item) => {
        if (seen.has(item.key)) return false;
        seen.add(item.key);
        return true;
    });
    return {
        items: [...state.items, ...additions],
        nextCursor: action.page.nextCursor,
        loadingMore: false,
    };
}
