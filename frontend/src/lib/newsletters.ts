import { api } from '@/lib/api';

export type NewsletterStatus = 'active' | 'stopped';

export type NewsletterFrequency =
    | 'new'
    | 'daily'
    | 'weekly'
    | 'biweekly'
    | 'monthly'
    | 'irregular'
    | 'noPosts';

export type NewsletterSource = {
    id: string;
    kind: 'newsletter';
    title: string;
    senderName?: string | null;
    senderAddress: string;
    primary: boolean;
    tags: string[];
    status: NewsletterStatus;
    lastReceivedAt?: string | null;
    frequency: NewsletterFrequency;
    issueCount: number;
    savedByYou: number;
};

export type NewsletterSources = {
    active: NewsletterSource[];
    stopped: NewsletterSource[];
};

export type NewsletterPatch = {
    title: string;
    primary: boolean;
    tags: string[];
};

export type NewsletterMessageMeta = {
    messageId: string;
    senderName: string | null;
    senderAddress: string;
    sentAt: string | null;
    hasBlockedRemoteImages: boolean;
    remoteImagesAllowed: boolean;
};

export function fetchNewsletters(signal?: AbortSignal) {
    return api<NewsletterSources>('/api/newsletters', { signal });
}

export function patchNewsletter(id: string, patch: NewsletterPatch) {
    return api<NewsletterSource>(`/api/newsletters/${encodeURIComponent(id)}`, {
        method: 'PATCH',
        body: patch,
    });
}

export function stopNewsletter(id: string) {
    return api<NewsletterSource>(
        `/api/newsletters/${encodeURIComponent(id)}/stop`,
        { method: 'POST' },
    );
}

export function enableNewsletter(id: string) {
    return api<NewsletterSource>(
        `/api/newsletters/${encodeURIComponent(id)}/enable`,
        { method: 'POST' },
    );
}

type NewsletterListener = () => void;
const newsletterListeners = new Set<NewsletterListener>();

export function emitNewsletterMutation(): void {
    for (const listener of newsletterListeners) listener();
}

export function subscribeNewsletterMutation(
    listener: NewsletterListener,
): () => void {
    newsletterListeners.add(listener);
    return () => newsletterListeners.delete(listener);
}
