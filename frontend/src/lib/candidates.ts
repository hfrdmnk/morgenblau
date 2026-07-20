export type FeedCandidate = {
    feedUrl?: string;
    kind?: 'standardfeed';
    publication?: string;
    title: string | null;
    siteUrl: string | null;
    // Set by the resolver when the user already subscribes to this site under the other kind (rss vs ATProto).
    subscribedVia?: { kind: string; title?: string };
};

// Same key the backend dedupes and subscribes on: publication at-uri for ATProto candidates, feed URL for rss ones.
export function candidateKey(candidate: FeedCandidate): string {
    return candidate.publication ?? candidate.feedUrl ?? '';
}
