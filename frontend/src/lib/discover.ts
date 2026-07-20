import { ApiError, type FieldErrors } from '@/lib/api';
import { truncateDid } from '@/lib/handle';
import type { Profile } from '@/lib/profile';
import { hostnameOf, safeHref } from '@/lib/utils';

// SPEC <discovery> Trust tiers: a person following via both reader-network and adjacent-graph counts once, at the strong tier.
export type DiscoverSignalKind = 'author' | 'subscribe' | 'share';

export type DiscoverSourceApp = 'skyreader' | 'glean';

export type DiscoverReason = {
    strongCount: number;
    weakCount: number;
    topFollowerDid?: string;
    // Set only when topFollowerDid is a weak-tier (adjacent-graph) follower.
    topFollowerNetwork?: 'bluesky' | 'tangled';
    // SPEC <discovery> Signal ordering (author > subscribe > share); absent, or unrecognized (e.g. a cached 'save' from before the save-privacy purge), normalizes to 'subscribe'.
    topSignal?: DiscoverSignalKind;
    // SPEC <discovery>: self-import outranks strong/weak signals at equal kind, overriding every other reason phrasing.
    selfSourceApp?: DiscoverSourceApp;
    followerDids?: string[];
    authorDid?: string;
    trending?: boolean;
};

export type DiscoverSourcePost = {
    key: string;
    title: string;
    publishedAt: string;
    url?: string;
};

export type PostsState =
    | { status: 'loading' }
    | { status: 'loaded'; posts: DiscoverSourcePost[] };

const SIGNAL_VERB: Record<DiscoverSignalKind, string> = {
    author: 'writes this',
    subscribe: 'subscribes',
    share: 'shared this',
};

const SIGNAL_FALLBACK_PHRASE: Record<DiscoverSignalKind, string> = {
    author: '1 person you follow writes this',
    subscribe: '1 person you follow subscribes',
    share: '1 person you follow shared this',
};

// Keyed by topSignal (strongest contributor) so a share-driven suggestion never claims "subscribe".
const SIGNAL_PLURAL_VERB: Record<DiscoverSignalKind, string> = {
    author: 'write this',
    subscribe: 'subscribe',
    share: 'shared this',
};

// A cached 'save' (or any other unrecognized value) from before the save-privacy purge falls back to subscribe.
function normalizeSignal(signal: DiscoverSignalKind | undefined): DiscoverSignalKind {
    return signal && signal in SIGNAL_VERB ? signal : 'subscribe';
}

// SPEC <discovery> Cards. Trending stays bare since it has no per-user contributor to credit.
export type DiscoverSourceCard = {
    key: string;
    kind: 'rss' | 'standardfeed';
    title?: string;
    siteUrl?: string;
    feedUrl?: string;
    publication?: string;
};

export type DiscoverSource = DiscoverSourceCard & {
    reason: DiscoverReason;
};

// Pure formatter: topFollowerLabel is caller-resolved, since DID-to-handle lookup is a network concern this function doesn't own.
// A weak-tier sole contributor names the network, not the person (SPEC <discovery>), since that follow lives outside the reader network.
export function formatDiscoverReason(
    reason: DiscoverReason,
    topFollowerLabel?: string,
): string {
    if (reason.selfSourceApp === 'skyreader') {
        return 'you subscribe on Skyreader';
    }
    if (reason.selfSourceApp === 'glean') {
        return 'you subscribe on Glean';
    }

    const total = reason.strongCount + reason.weakCount;

    if (total === 1 && reason.topFollowerNetwork === 'bluesky') {
        return 'you follow on Bluesky';
    }
    if (total === 1 && reason.topFollowerNetwork === 'tangled') {
        return 'you follow on Tangled';
    }
    const signal = normalizeSignal(reason.topSignal);
    if (total === 1 && topFollowerLabel) {
        return `${topFollowerLabel} ${SIGNAL_VERB[signal]}`;
    }
    if (total === 1) {
        return SIGNAL_FALLBACK_PHRASE[signal];
    }
    return `${total} people you follow ${SIGNAL_PLURAL_VERB[signal]}`;
}

export type DiscoverReasonDisplay =
    | { kind: 'text'; text: string }
    | { kind: 'author'; did: string }
    | { kind: 'people'; followerDids: string[]; total: number }
    | { kind: 'trending' };

// SPEC <discovery> Cards: which reason shape to render, in the same precedence as formatDiscoverReason (self always wins).
export function resolveDiscoverReasonDisplay(
    reason: DiscoverReason,
): DiscoverReasonDisplay {
    if (reason.selfSourceApp) {
        return { kind: 'text', text: formatDiscoverReason(reason) };
    }
    const total = reason.strongCount + reason.weakCount;
    if (reason.topSignal === 'author' && total === 1 && reason.authorDid) {
        return { kind: 'author', did: reason.authorDid };
    }
    if (total > 0) {
        return {
            kind: 'people',
            followerDids: (reason.followerDids ?? []).slice(0, 3),
            total,
        };
    }
    if (reason.trending) {
        return { kind: 'trending' };
    }
    return { kind: 'text', text: 'New to your network' };
}

// Fan-out list for the caller's profile lookup; a follower may appear in more than one field.
export function discoverReasonDids(reason: DiscoverReason): string[] {
    return Array.from(
        new Set(
            [
                reason.authorDid,
                reason.topFollowerDid,
                ...(reason.followerDids ?? []),
            ].filter((did): did is string => Boolean(did)),
        ),
    );
}

export function discoverFollowerLabel(
    did: string,
    profile: Profile | undefined,
): string {
    return profile?.handle ? `@${profile.handle}` : truncateDid(did);
}

// Standardfeed sends a blank title so no blue.morgen sidecar record is created (untouched defaults mean no sidecar, per the add-dialog convention).
export function discoverSubscribePayload(
    source: DiscoverSourceCard,
): Record<string, unknown> {
    if (source.kind === 'standardfeed') {
        return {
            publication: source.publication,
            title: '',
            primary: false,
            tags: [],
        };
    }
    return {
        feedUrl: source.feedUrl,
        title: source.title ?? '',
        siteUrl: source.siteUrl ?? '',
        primary: false,
        tags: [],
    };
}

// source.key is already the canonical key the server hide store dedupes on.
export function discoverHidePayload(
    source: DiscoverSourceCard,
): { targetKind: 'source'; targetKey: string } {
    return { targetKind: 'source', targetKey: source.key };
}

export function discoverSourceTitle(source: DiscoverSourceCard): string {
    return (
        source.title ||
        (source.siteUrl && hostnameOf(source.siteUrl)) ||
        (source.feedUrl && hostnameOf(source.feedUrl)) ||
        source.publication ||
        source.key
    );
}

export function discoverSourceFaviconUrl(source: DiscoverSourceCard): string {
    return `/api/favicon?feed=${encodeURIComponent(source.feedUrl ?? source.key)}`;
}

function feedOrigin(feedUrl: string | undefined): string | undefined {
    if (!feedUrl) return undefined;
    try {
        return new URL(feedUrl).origin;
    } catch {
        return undefined;
    }
}

// Never links the raw feed URL (renders XML); origin-of-feedUrl mirrors the backend's RSS favicon-site fallback.
export function discoverSourceLinkHref(
    source: DiscoverSourceCard,
): string | undefined {
    return safeHref(source.siteUrl) ?? safeHref(feedOrigin(source.feedUrl));
}

// Subscribe-dialog payload: identity fields come from discoverSubscribePayload, editable fields from the form.
// Standardfeed still sends a blank title when the user left the dialog's prefill unchanged (add-dialog convention).
export function discoverSubscribeOverridePayload(
    source: DiscoverSourceCard,
    override: { title: string; primary: boolean; tags: string[] },
): Record<string, unknown> {
    const title = override.title.trim();
    const prefilled = discoverSourceTitle(source).trim();
    const unchanged = title === prefilled;
    return {
        ...discoverSubscribePayload(source),
        title: source.kind === 'standardfeed' && unchanged ? '' : title,
        primary: override.primary,
        tags: override.tags,
    };
}

// Prefers an error on a non-editable identity field or the general list, then the submit failure message.
export function pickSubscribeTopLevelError(
    fieldErrors: FieldErrors,
    submitError: string | undefined,
): string | undefined {
    return (
        fieldErrors['subscriptions.0.feedUrl'] ??
        fieldErrors['subscriptions.0.publication'] ??
        fieldErrors.subscriptions ??
        submitError
    );
}

export type SubscribeErrorResult =
    | { kind: 'reauth' }
    | { kind: 'fields'; errors: FieldErrors }
    | { kind: 'message'; message: string };

// Collapses a subscribe-POST failure into the three ways the subscribe dialog surfaces it.
export function classifySubscribeError(
    error: unknown,
    fallback = 'Couldn’t add that source. Try again?',
): SubscribeErrorResult {
    if (!(error instanceof ApiError)) {
        return { kind: 'message', message: fallback };
    }
    if (error.isReauth) {
        return { kind: 'reauth' };
    }
    if (error.errors) {
        return { kind: 'fields', errors: error.errors };
    }
    return { kind: 'message', message: error.message };
}
