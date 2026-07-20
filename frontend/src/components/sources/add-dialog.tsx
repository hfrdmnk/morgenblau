import { SpinnerIcon } from '@proicons/react';
import type { FormEvent, KeyboardEvent } from 'react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import { FeedCandidateList } from '@/components/sources/feed-candidate-list';
import { ApiError, api } from '@/lib/api';
import { candidateKey, type FeedCandidate } from '@/lib/candidates';
import { InputError } from '@/components/input-error';
import { ReauthNotice } from '@/components/reauth-notice';
import {
    emitSubscriptionAdded,
    type AddedSubscription,
} from '@/lib/subscription-events';
import { mergeTagSuggestions } from '@/lib/tags';
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
import { youtubeShortsFreeFeedUrl } from '@/lib/youtube';

// feedUrl is the catalog key: feed URL for rss subscriptions, publication at-uri for ATProto.
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
    // User-editable title, prefilled from the resolver (blue.morgen.feed.subscription.title).
    title: string;
    // ATProto items only send title when it diverged from prefill; unchanged means no sidecar record.
    prefilledTitle: string;
    siteUrl: string;
    primary: boolean;
    tags: string[];
    // UI-only: submits the Shorts-free playlist feed instead of the channel feed at submit.
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

// siblingKeyOf mirrors the server's sibling rule: lowercase host minus "www."
// plus path minus trailing slash. rss candidates fall back to the feed URL's host.
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
            const result = await api<DiscoverResult>(
                '/api/subscriptions/resolve',
                { method: 'POST', body: { url }, signal: abort.signal },
            );
            setCandidates(result.candidates);
            setExistingSubscriptions(result.existingSubscriptions);

            const existing = new Set(
                result.existingSubscriptions.map((s) => s.feedUrl),
            );
            // Fresh = not already subscribed and not a cross-kind sibling of an existing subscription.
            const fresh = result.candidates.filter(
                (c) => !existing.has(candidateKey(c)) && !c.subscribedVia,
            );

            if (fresh.length === 1) {
                setSubscriptions([toItem(fresh[0])]);
            }
        } catch (err) {
            if (err instanceof ApiError) {
                setDiscoverError(
                    err.status >= 500
                        ? 'Couldn’t reach that URL. Try again?'
                        : err.message,
                );
                return;
            }
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
        api<{ tags?: string[] }>('/api/subscriptions/tags')
            .then((body) => {
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

    // Blocked while a cross-kind sibling (same site) is selected, to avoid duplicate subscriptions.
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

    // ATProto subscribes need the post-change OAuth grant, so surface the re-auth note once selected.
    const hasStandardSelected = subscriptions.some(
        (item) => item.kind === 'standardfeed',
    );
    const [needsReauth, setNeedsReauth] = useState(false);
    useEffect(() => {
        if (!hasStandardSelected) {
            return;
        }
        let active = true;
        api<{ needsReauth?: boolean }>('/api/profiles/me')
            .then((body) => {
                if (active && body) {
                    setNeedsReauth(body.needsReauth === true);
                }
            })
            .catch(() => {});
        return () => {
            active = false;
        };
    }, [hasStandardSelected]);

    // Tag suggestions = user's existing tags ∪ tags added in this dialog, deduped case-insensitively.
    const tagSuggestions = useMemo(
        () =>
            mergeTagSuggestions(
                userTags,
                subscriptions.flatMap((item) => item.tags),
            ),
        [userTags, subscriptions],
    );

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
                // Send title only when it diverged from the prefill; untouched defaults create no sidecar record.
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
            const body = await api<{
                records?: AddedSubscription[];
                jobIds?: string[];
            }>('/api/subscriptions', {
                method: 'POST',
                body: { subscriptions: payload },
            });
            emitSubscriptionAdded({
                records: body?.records ?? [],
                jobIds: body?.jobIds ?? [],
            });

            resetState();
            onOpenChange(false);
        } catch (err) {
            if (err instanceof ApiError) {
                if (err.isReauth) {
                    setNeedsReauth(true);
                    setSubmitError(undefined);
                    return;
                }
                if (err.errors) {
                    setFieldErrors(err.errors);
                    return;
                }
                setSubmitError(err.message);
                return;
            }
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
                                        <SpinnerIcon className="motion-safe:animate-spin" />
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

                    {hasStandardSelected && needsReauth && <ReauthNotice />}

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
                                    <SpinnerIcon className="motion-safe:animate-spin" />
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
