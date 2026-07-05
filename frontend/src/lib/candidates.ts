export type FeedCandidate = {
    feedUrl?: string;
    kind?: 'standardfeed';
    publication?: string;
    title: string | null;
    siteUrl: string | null;
    // Set by the resolver when the user already subscribes to this site
    // under the other kind (rss vs ATProto).
    subscribedVia?: { kind: string; title?: string };
};

// candidateKey is the candidate's identity everywhere in the picker: the
// publication at-uri for ATProto candidates, the feed URL for rss ones —
// the same key the backend dedupes and subscribes on.
export function candidateKey(candidate: FeedCandidate): string {
    return candidate.publication ?? candidate.feedUrl ?? '';
}
