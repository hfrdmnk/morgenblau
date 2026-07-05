import { Loading03Icon } from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import type { FormEvent, KeyboardEvent } from 'react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import { FeedCandidateList } from '@/components/sources/feed-candidate-list';
import { candidateKey, type FeedCandidate } from '@/lib/candidates';
import { InputError } from '@/components/input-error';
import {
    emitSubscriptionAdded,
    type AddedSubscription,
} from '@/lib/subscription-events';
import { Button } from '@/components/ui/button';
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { PATHS } from '@/lib/paths';
import { youtubeShortsFreeFeedUrl } from '@/lib/youtube';

// feedUrl carries the catalog key: the feed URL for rss subscriptions, the
// publication at-uri for ATProto ones.
type ExistingFeedSubscription = { feedUrl: string; title: string | null };

type DiscoverResult = {
    candidates: FeedCandidate[];
    existingSubscriptions: ExistingFeedSubscription[];
};

type SubscriptionItem = {
    // Candidate identity: publication at-uri or feed URL.
    key: string;
    kind: 'rss' | 'standardfeed';
    feedUrl: string;
    publication?: string;
    // User-editable display title. Prefilled from the resolver, then owned by
    // the user (lexicon: blue.morgen.feed.subscription `title` is user-editable).
    title: string;
    // The resolver's prefill: an ATProto item only sends its title when the
    // user diverged from it (unchanged defaults mean no sidecar record).
    prefilledTitle: string;
    siteUrl: string;
    primary: boolean;
    tags: string[];
    // UI-only: when true, submit the Shorts-free YouTube playlist feed instead
    // of the channel feed. Resolved to the effective feedUrl at submit.
    excludeShorts: boolean;
};

type Props = {
    open: boolean;
    onOpenChange: (open: boolean) => void;
};

function toItem(candidate: FeedCandidate): SubscriptionItem {
    return {
        key: candidateKey(candidate),
        kind: candidate.kind === 'standardfeed' ? 'standardfeed' : 'rss',
        feedUrl: candidate.feedUrl ?? '',
        publication: candidate.publication,
        title: candidate.title ?? '',
        prefilledTitle: candidate.title ?? '',
        siteUrl: candidate.siteUrl ?? '',
        primary: false,
        tags: [],
        excludeShorts: false,
    };
}

// siblingKeyOf mirrors the server's sibling rule: lowercase host minus
// "www." plus the path minus its trailing slash. rss candidates fall back
// to the feed URL's bare host when no site URL is known.
function siblingKeyOf(candidate: FeedCandidate): string | null {
    const normalized = normalizeSiblingUrl(candidate.siteUrl ?? '');
    if (candidate.kind === 'standardfeed') {
        return normalized;
    }
    if (normalized) {
        return normalized;
    }
    try {
        return new URL(candidate.feedUrl ?? '').hostname
            .toLowerCase()
            .replace(/^www\./, '');
    } catch {
        return null;
    }
}

function normalizeSiblingUrl(raw: string): string | null {
    if (!raw) return null;
    try {
        const u = new URL(raw);
        return (
            u.hostname.toLowerCase().replace(/^www\./, '') +
            u.pathname.replace(/\/+$/, '')
        );
    } catch {
        return null;
    }
}

export function AddSourceDialog({ open, onOpenChange }: Props) {
    const [url, setUrl] = useState('');
    const [discovering, setDiscovering] = useState(false);
    const [discoverError, setDiscoverError] = useState<string | undefined>(
        undefined,
    );
    const discoverAbortRef = useRef<AbortController | null>(null);

    const [candidates, setCandidates] = useState<FeedCandidate[] | null>(null);
    const [existingSubscriptions, setExistingSubscriptions] = useState<
        ExistingFeedSubscription[]
    >([]);

    const [subscriptions, setSubscriptions] = useState<SubscriptionItem[]>([]);
    const [submitting, setSubmitting] = useState(false);
    const [submitError, setSubmitError] = useState<string | undefined>(
        undefined,
    );
    const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
    const [userTags, setUserTags] = useState<string[]>([]);

    const firstCheckboxRef = useRef<HTMLInputElement>(null);
    const firstTitleInputRef = useRef<HTMLInputElement>(null);
    const previousCandidatesRef = useRef<FeedCandidate[] | null>(null);

    const resetState = useCallback(() => {
        discoverAbortRef.current?.abort();
        discoverAbortRef.current = null;
        setUrl('');
        setDiscovering(false);
        setDiscoverError(undefined);
        setCandidates(null);
        setExistingSubscriptions([]);
        setSubscriptions([]);
        setSubmitting(false);
        setSubmitError(undefined);
        setFieldErrors({});
    }, []);

    const handleOpenChangeComplete = (nextOpen: boolean) => {
        if (nextOpen) {
            return;
        }

        resetState();
    };

    const onUrlChange = (next: string) => {
        setUrl(next);

        if (candidates !== null) {
            setCandidates(null);
            setExistingSubscriptions([]);
            setDiscoverError(undefined);
            setSubscriptions([]);
            setFieldErrors({});
        }
    };

    const findFeeds = useCallback(async () => {
        if (!url.trim()) {
            return;
        }

        discoverAbortRef.current?.abort();
        const abort = new AbortController();
        discoverAbortRef.current = abort;
        setDiscoverError(undefined);
        setDiscovering(true);

        try {
            const response = await fetch('/api/subscriptions/resolve', {
                method: 'POST',
                headers: { 'content-type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify({ url }),
                signal: abort.signal,
            });

            if (!response.ok) {
                if (response.status >= 500) {
                    setDiscoverError('Couldn’t reach that URL. Try again?');
                } else {
                    const body = (await response
                        .json()
                        .catch(() => null)) as { message?: string } | null;
                    setDiscoverError(body?.message ?? 'Discovery failed.');
                }
                return;
            }

            const result = (await response.json()) as DiscoverResult;
            setCandidates(result.candidates);
            setExistingSubscriptions(result.existingSubscriptions);

            const existing = new Set(
                result.existingSubscriptions.map((s) => s.feedUrl),
            );
            // Fresh = not already subscribed, and not a sibling of an
            // existing subscription of the other kind.
            const fresh = result.candidates.filter(
                (c) => !existing.has(candidateKey(c)) && !c.subscribedVia,
            );

            if (fresh.length === 1) {
                setSubscriptions([toItem(fresh[0])]);
            }
        } catch (err) {
            if ((err as Error).name === 'AbortError') return;
            setDiscoverError('Couldn’t reach that URL. Try again?');
        } finally {
            if (discoverAbortRef.current === abort) {
                discoverAbortRef.current = null;
            }
            setDiscovering(false);
        }
    }, [url]);

    const onUrlKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
        if (event.key !== 'Enter' || event.nativeEvent.isComposing) {
            return;
        }

        if (candidates !== null || !url.trim() || discovering) {
            return;
        }

        event.preventDefault();
        findFeeds();
    };

    const hasCandidates = candidates !== null && candidates.length > 0;
    const selectedCount = subscriptions.length;
    const submitDisabled = submitting || selectedCount === 0;

    useEffect(() => {
        const previous = previousCandidatesRef.current;
        previousCandidatesRef.current = candidates;

        if (previous !== null || candidates === null) {
            return;
        }

        if (subscriptions.length === 1) {
            const titleInput = firstTitleInputRef.current;

            if (titleInput) {
                titleInput.focus();
                titleInput.select();

                return;
            }
        }

        firstCheckboxRef.current?.focus();
    }, [candidates, subscriptions.length]);

    // Pull the user's existing tags once per open, to seed tag suggestions.
    useEffect(() => {
        if (!open) {
            return;
        }
        let active = true;
        fetch('/api/subscriptions/tags', { credentials: 'same-origin' })
            .then((response) => (response.ok ? response.json() : null))
            .then((body: { tags?: string[] } | null) => {
                if (active && body?.tags) {
                    setUserTags(body.tags);
                }
            })
            .catch(() => { });
        return () => {
            active = false;
        };
    }, [open]);

    const selectedMap = useMemo(
        () =>
            Object.fromEntries(
                subscriptions.map((item) => [
                    item.key,
                    {
                        title: item.title,
                        primary: item.primary,
                        tags: item.tags,
                        excludeShorts: item.excludeShorts,
                    },
                ]),
            ),
        [subscriptions],
    );

    // A candidate is blocked while its cross-kind sibling (same site, other
    // kind) is selected — subscribing to both would duplicate the source.
    const siblingBlocked = useMemo(() => {
        const blocked = new Set<string>();
        if (!candidates) return blocked;
        for (const candidate of candidates) {
            const key = candidateKey(candidate);
            if (selectedMap[key]) continue;
            const ownKind = candidate.kind ?? 'rss';
            const ownSibling = siblingKeyOf(candidate);
            if (!ownSibling) continue;
            const hasSelectedSibling = candidates.some(
                (other) =>
                    (other.kind ?? 'rss') !== ownKind &&
                    siblingKeyOf(other) === ownSibling &&
                    selectedMap[candidateKey(other)] !== undefined,
            );
            if (hasSelectedSibling) blocked.add(key);
        }
        return blocked;
    }, [candidates, selectedMap]);

    // ATProto subscribes need the post-change OAuth grant; surface the calm
    // re-auth note as soon as an ATProto candidate is selected.
    const hasStandardSelected = subscriptions.some(
        (item) => item.kind === 'standardfeed',
    );
    const [needsReauth, setNeedsReauth] = useState(false);
    useEffect(() => {
        if (!hasStandardSelected) {
            return;
        }
        let active = true;
        fetch('/api/profiles/me', { credentials: 'same-origin' })
            .then((response) => (response.ok ? response.json() : null))
            .then((body: { needsReauth?: boolean } | null) => {
                if (active && body) {
                    setNeedsReauth(body.needsReauth === true);
                }
            })
            .catch(() => {});
        return () => {
            active = false;
        };
    }, [hasStandardSelected]);

    // Suggestions = tags the user already uses ∪ tags added elsewhere in this
    // dialog, deduped case-insensitively and sorted.
    const tagSuggestions = useMemo(() => {
        const byLower = new Map<string, string>();
        for (const tag of userTags) {
            const key = tag.toLowerCase();
            if (!byLower.has(key)) byLower.set(key, tag);
        }
        for (const item of subscriptions) {
            for (const tag of item.tags) {
                const key = tag.toLowerCase();
                if (!byLower.has(key)) byLower.set(key, tag);
            }
        }
        return [...byLower.values()].sort((a, b) => a.localeCompare(b));
    }, [userTags, subscriptions]);

    const existingByFeedUrl = useMemo(
        () =>
            new Map(
                existingSubscriptions.map(
                    (s) => [s.feedUrl, s.title] as const,
                ),
            ),
        [existingSubscriptions],
    );

    const toggleCandidate = useCallback((candidate: FeedCandidate) => {
        const key = candidateKey(candidate);
        setSubscriptions((current) => {
            const exists = current.some((item) => item.key === key);

            return exists
                ? current.filter((item) => item.key !== key)
                : [...current, toItem(candidate)];
        });
    }, []);

    const handleTitleChange = useCallback((key: string, title: string) => {
        setSubscriptions((current) =>
            current.map((item) =>
                item.key === key ? { ...item, title } : item,
            ),
        );
    }, []);

    const handlePrimaryChange = useCallback(
        (key: string, primary: boolean) => {
            setSubscriptions((current) =>
                current.map((item) =>
                    item.key === key ? { ...item, primary } : item,
                ),
            );
        },
        [],
    );

    const handleTagsChange = useCallback((key: string, tags: string[]) => {
        setSubscriptions((current) =>
            current.map((item) =>
                item.key === key ? { ...item, tags } : item,
            ),
        );
    }, []);

    const handleExcludeShortsChange = useCallback(
        (key: string, excludeShorts: boolean) => {
            setSubscriptions((current) =>
                current.map((item) =>
                    item.key === key ? { ...item, excludeShorts } : item,
                ),
            );
        },
        [],
    );

    const submit = async (event: FormEvent) => {
        event.preventDefault();

        if (submitting) {
            return;
        }

        if (!hasCandidates) {
            if (!url.trim() || discovering) {
                return;
            }
            findFeeds();
            return;
        }

        if (selectedCount === 0) {
            return;
        }

        setSubmitting(true);
        setSubmitError(undefined);
        setFieldErrors({});

        const payload = subscriptions.map((item) => {
            if (item.kind === 'standardfeed') {
                // Send metadata only when the user diverged from the
                // prefill — untouched defaults mean no sidecar record.
                const title = item.title.trim();
                return {
                    publication: item.publication,
                    siteUrl: item.siteUrl,
                    title:
                        title === item.prefilledTitle.trim() ? '' : title,
                    primary: item.primary,
                    tags: item.tags,
                };
            }
            const feedUrl = item.excludeShorts
                ? (youtubeShortsFreeFeedUrl(item.feedUrl) ?? item.feedUrl)
                : item.feedUrl;
            return {
                feedUrl,
                title: item.title.trim(),
                siteUrl: item.siteUrl,
                primary: item.primary,
                tags: item.tags,
            };
        });

        try {
            const response = await fetch('/api/subscriptions', {
                method: 'POST',
                headers: { 'content-type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify({ subscriptions: payload }),
            });

            if (!response.ok) {
                const body = (await response.json().catch(() => null)) as {
                    errors?: Record<string, string>;
                    message?: string;
                    code?: string;
                } | null;
                if (body?.code === 'reauth_required') {
                    setNeedsReauth(true);
                    setSubmitError(undefined);
                    return;
                }
                if (body?.errors) {
                    setFieldErrors(body.errors);
                }
                if (body?.message) {
                    setSubmitError(body.message);
                } else if (!body?.errors) {
                    setSubmitError('Couldn’t add those sources. Try again?');
                }
                return;
            }

            const body = (await response.json().catch(() => null)) as {
                records?: AddedSubscription[];
                jobIds?: string[];
            } | null;
            emitSubscriptionAdded({
                records: body?.records ?? [],
                jobIds: body?.jobIds ?? [],
            });

            resetState();
            onOpenChange(false);
        } catch {
            setSubmitError('Couldn’t add those sources. Try again?');
        } finally {
            setSubmitting(false);
        }
    };

    const submitLabel =
        selectedCount > 1 ? `Add ${selectedCount} sources` : 'Add source';

    const topLevelError = fieldErrors.subscriptions ?? submitError;

    return (
        <Dialog
            open={open}
            onOpenChange={onOpenChange}
            onOpenChangeComplete={handleOpenChangeComplete}
        >
            <DialogContent className="max-h-[90dvh] grid-rows-[auto_minmax(0,1fr)] overflow-hidden">
                <DialogHeader>
                    <DialogTitle>Add a source</DialogTitle>
                    <DialogDescription>
                        Paste a website, RSS feed, or YouTube channel.
                    </DialogDescription>
                </DialogHeader>

                <form
                    onSubmit={submit}
                    noValidate
                    className="flex min-h-0 min-w-0 flex-col gap-5"
                >
                    <div className="space-y-2">
                        <Label htmlFor="source-url" className="sr-only">
                            URL
                        </Label>
                        <div className="flex items-center gap-2">
                            <Input
                                id="source-url"
                                type="text"
                                inputMode="url"
                                enterKeyHint="search"
                                autoFocus
                                spellCheck={false}
                                placeholder="https://example.com"
                                value={url}
                                onChange={(event) =>
                                    onUrlChange(event.target.value)
                                }
                                onKeyDown={onUrlKeyDown}
                                aria-invalid={
                                    discoverError ? true : undefined
                                }
                                className="flex-1"
                            />
                            <Button
                                type="button"
                                variant="secondary"
                                onClick={findFeeds}
                                disabled={
                                    discovering ||
                                    !url.trim() ||
                                    candidates !== null
                                }
                            >
                                {discovering ? (
                                    <>
                                        <HugeiconsIcon
                                            icon={Loading03Icon}
                                            className="motion-safe:animate-spin"
                                        />
                                        Finding…
                                    </>
                                ) : (
                                    'Find'
                                )}
                            </Button>
                        </div>
                        {discoverError && (
                            <InputError message={discoverError} />
                        )}
                        <p className="text-sm font-light text-muted-foreground">
                            Your sources are currently all public.
                            (Selective private sources are coming.)
                        </p>
                    </div>

                    {hasCandidates && (
                        <>
                            <h2 id="candidate-list-heading" className="sr-only">
                                What we found
                            </h2>
                            <div className="-mx-6 min-h-0 flex-1 overflow-y-auto px-6">
                                <FeedCandidateList
                                    aria-labelledby="candidate-list-heading"
                                    candidates={candidates}
                                    existingByFeedUrl={existingByFeedUrl}
                                    siblingBlocked={siblingBlocked}
                                    selected={selectedMap}
                                    onToggle={toggleCandidate}
                                    onTitleChange={handleTitleChange}
                                    onPrimaryChange={handlePrimaryChange}
                                    onTagsChange={handleTagsChange}
                                    onExcludeShortsChange={
                                        handleExcludeShortsChange
                                    }
                                    tagSuggestions={tagSuggestions}
                                    firstCheckboxRef={firstCheckboxRef}
                                    firstTitleInputRef={firstTitleInputRef}
                                />
                            </div>
                        </>
                    )}

                    {hasCandidates &&
                        subscriptions.map((item, index) => {
                            const indexedErrors = [
                                fieldErrors[
                                `subscriptions.${index}.feedUrl`
                                ],
                                fieldErrors[
                                `subscriptions.${index}.publication`
                                ],
                                fieldErrors[
                                `subscriptions.${index}.title`
                                ],
                            ].filter(Boolean) as string[];

                            if (indexedErrors.length === 0) {
                                return null;
                            }

                            return (
                                <div
                                    key={item.key}
                                    className="flex flex-col gap-1"
                                >
                                    <p className="text-xs text-muted-foreground">
                                        {item.title || item.key}
                                    </p>
                                    {indexedErrors.map((message) => (
                                        <InputError
                                            key={message}
                                            message={message}
                                        />
                                    ))}
                                </div>
                            );
                        })}

                    {hasStandardSelected && needsReauth && (
                        <p
                            role="status"
                            className="text-sm font-light text-muted-foreground"
                        >
                            Subscribing via ATProto needs one extra
                            permission.{' '}
                            <a
                                href={PATHS.login}
                                className="text-primary underline underline-offset-4"
                            >
                                Sign in again
                            </a>{' '}
                            to enable it.
                        </p>
                    )}

                    {topLevelError && <InputError message={topLevelError} />}

                    <DialogFooter>
                        <Button
                            type="button"
                            variant="secondary"
                            onClick={() => onOpenChange(false)}
                            disabled={submitting}
                        >
                            Cancel
                        </Button>
                        <Button type="submit" disabled={submitDisabled}>
                            {submitting ? (
                                <>
                                    <HugeiconsIcon
                                        icon={Loading03Icon}
                                        className="motion-safe:animate-spin"
                                    />
                                    Adding…
                                </>
                            ) : (
                                submitLabel
                            )}
                        </Button>
                    </DialogFooter>
                </form>
            </DialogContent>
        </Dialog>
    );
}
