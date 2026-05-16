export type SubscriptionAddedEvent = {
    records: AddedSubscription[];
    jobIds: string[];
};

export type AddedSubscription = {
    uri: string;
    cid?: string;
    rkey: string;
    feedUrl: string;
    title?: string;
    siteUrl?: string;
    value: Record<string, unknown>;
};

type Listener = (event: SubscriptionAddedEvent) => void;

const listeners = new Set<Listener>();

export function emitSubscriptionAdded(event: SubscriptionAddedEvent): void {
    for (const listener of listeners) {
        listener(event);
    }
}

export function subscribeSubscriptionAdded(listener: Listener): () => void {
    listeners.add(listener);
    return () => {
        listeners.delete(listener);
    };
}
