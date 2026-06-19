import { Loading03Icon } from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import type { FormEvent, KeyboardEvent } from 'react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import {
    FeedCandidateList,
    type FeedCandidate,
} from '@/components/sources/feed-candidate-list';
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
import { youtubeShortsFreeFeedUrl } from '@/lib/youtube';

type ExistingFeedSubscription = { feedUrl: string; title: string | null };

type DiscoverResult = {
    candidates: FeedCandidate[];
    existingSubscriptions: ExistingFeedSubscription[];
};

type SubscriptionItem = {
    feedUrl: string;
    // User-editable display title. Prefilled from the resolver, then owned by
    // the user (lexicon: blue.morgen.feed.subscription `title` is user-editable).
    title: string;
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
        feedUrl: candidate.feedUrl,
        title: candidate.title ?? '',
        siteUrl: candidate.siteUrl ?? '',
        primary: false,
        tags: [],
        excludeShorts: false,
    };
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
            const fresh = result.candidates.filter(
                (c) => !existing.has(c.feedUrl),
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
            .catch(() => {});
        return () => {
            active = false;
        };
    }, [open]);

    const selectedMap = useMemo(
        () =>
            Object.fromEntries(
                subscriptions.map((item) => [
                    item.feedUrl,
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
        setSubscriptions((current) => {
            const exists = current.some(
                (item) => item.feedUrl === candidate.feedUrl,
            );

            return exists
                ? current.filter(
                      (item) => item.feedUrl !== candidate.feedUrl,
                  )
                : [...current, toItem(candidate)];
        });
    }, []);

    const handleTitleChange = useCallback(
        (feedUrl: string, title: string) => {
            setSubscriptions((current) =>
                current.map((item) =>
                    item.feedUrl === feedUrl ? { ...item, title } : item,
                ),
            );
        },
        [],
    );

    const handlePrimaryChange = useCallback(
        (feedUrl: string, primary: boolean) => {
            setSubscriptions((current) =>
                current.map((item) =>
                    item.feedUrl === feedUrl ? { ...item, primary } : item,
                ),
            );
        },
        [],
    );

    const handleTagsChange = useCallback((feedUrl: string, tags: string[]) => {
        setSubscriptions((current) =>
            current.map((item) =>
                item.feedUrl === feedUrl ? { ...item, tags } : item,
            ),
        );
    }, []);

    const handleExcludeShortsChange = useCallback(
        (feedUrl: string, excludeShorts: boolean) => {
            setSubscriptions((current) =>
                current.map((item) =>
                    item.feedUrl === feedUrl
                        ? { ...item, excludeShorts }
                        : item,
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
                } | null;
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
                        Paste a website, RSS feed, YouTube channel, or podcast
                        URL.
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
                                    `subscriptions.${index}.title`
                                ],
                            ].filter(Boolean) as string[];

                            if (indexedErrors.length === 0) {
                                return null;
                            }

                            return (
                                <div
                                    key={item.feedUrl}
                                    className="flex flex-col gap-1"
                                >
                                    <p className="text-xs text-muted-foreground">
                                        {item.title || item.feedUrl}
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
