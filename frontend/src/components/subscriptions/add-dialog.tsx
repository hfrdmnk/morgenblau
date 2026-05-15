import { Loading03Icon } from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import type { FormEvent, KeyboardEvent } from 'react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import InputError from '@/components/input-error';
import {
    FeedCandidateList,
    type FeedCandidate,
} from '@/components/subscriptions/feed-candidate-list';
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
import { isMacPlatform } from '@/lib/utils';

type ExistingSubscription = { feed_url: string; title: string | null };

type DiscoverResult = {
    candidates: FeedCandidate[];
    existing_subscriptions: ExistingSubscription[];
};

type SubscriptionItem = {
    feed_url: string;
    title: string;
    site_url: string;
};

type Props = {
    open: boolean;
    onOpenChange: (open: boolean) => void;
};

function toItem(candidate: FeedCandidate): SubscriptionItem {
    return {
        feed_url: candidate.feed_url,
        title: candidate.title ?? '',
        site_url: candidate.site_url ?? '',
    };
}

export function AddSubscriptionDialog({ open, onOpenChange }: Props) {
    // Discovery state.
    const [url, setUrl] = useState('');
    const [discovering, setDiscovering] = useState(false);
    const [discoverError, setDiscoverError] = useState<string | undefined>(
        undefined,
    );
    const discoverAbortRef = useRef<AbortController | null>(null);

    // Result state.
    const [candidates, setCandidates] = useState<FeedCandidate[] | null>(null);
    const [existingSubscriptions, setExistingSubscriptions] = useState<
        ExistingSubscription[]
    >([]);

    // Submit state.
    const [subscriptions, setSubscriptions] = useState<SubscriptionItem[]>([]);
    const [submitting, setSubmitting] = useState(false);
    const [submitError, setSubmitError] = useState<string | undefined>(
        undefined,
    );
    const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});

    const firstCheckboxRef = useRef<HTMLInputElement>(null);
    const firstTitleInputRef = useRef<HTMLInputElement>(null);
    const previousCandidatesRef = useRef<FeedCandidate[] | null>(null);

    const isMac = useMemo(() => isMacPlatform(), []);

    const close = () => {
        onOpenChange(false);
    };

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
            // TODO(backend): expose `POST /api/subscriptions/discover` accepting
            // `{ url: string }` and returning
            // `{ candidates: FeedCandidate[], existing_subscriptions: ExistingSubscription[] }`.
            // Server resolves the URL, scrapes for feeds, and reports which are
            // already in the user's atproto subscription set.
            const response = await fetch('/api/subscriptions/discover', {
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
            setExistingSubscriptions(result.existing_subscriptions);

            const existing = new Set(
                result.existing_subscriptions.map((s) => s.feed_url),
            );
            const fresh = result.candidates.filter(
                (c) => !existing.has(c.feed_url),
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

    const onFormKeyDown = (event: KeyboardEvent<HTMLFormElement>) => {
        if (event.key !== 'Enter' || event.nativeEvent.isComposing) {
            return;
        }

        if (event.defaultPrevented) {
            return;
        }

        const target = event.target;
        const onSubmitButton =
            target instanceof HTMLButtonElement && target.type === 'submit';

        if (onSubmitButton) {
            return;
        }

        if (event.metaKey || event.ctrlKey) {
            event.preventDefault();

            if (hasCandidates && selectedCount > 0 && !submitting) {
                event.currentTarget.requestSubmit();
            }

            return;
        }

        if (target instanceof HTMLInputElement) {
            const isUrlInput = target.id === 'subscription-url';
            const isCheckbox = target.type === 'checkbox';

            if (isUrlInput || isCheckbox) {
                event.preventDefault();
            }
        }
    };

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

    const selectedMap = useMemo(
        () =>
            Object.fromEntries(
                subscriptions.map((item) => [
                    item.feed_url,
                    { title: item.title },
                ]),
            ),
        [subscriptions],
    );

    const existingByFeedUrl: Record<string, string | null> = useMemo(
        () =>
            Object.fromEntries(
                existingSubscriptions.map((s) => [s.feed_url, s.title]),
            ),
        [existingSubscriptions],
    );

    const toggleCandidate = useCallback((candidate: FeedCandidate) => {
        setSubscriptions((current) => {
            const exists = current.some(
                (item) => item.feed_url === candidate.feed_url,
            );

            return exists
                ? current.filter(
                      (item) => item.feed_url !== candidate.feed_url,
                  )
                : [...current, toItem(candidate)];
        });
    }, []);

    const handleTitleChange = useCallback(
        (feedUrl: string, title: string) => {
            setSubscriptions((current) =>
                current.map((item) =>
                    item.feed_url === feedUrl ? { ...item, title } : item,
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

        try {
            // TODO(backend): expose `POST /api/subscriptions` accepting
            // `{ subscriptions: [{ feed_url, title, site_url }] }`.
            // Server writes one `app.skyreader.subscription` record per item to
            // the user's atproto repo. On 422, return
            // `{ errors: { "subscriptions.0.title": "...", message?: "..." } }`.
            const response = await fetch('/api/subscriptions', {
                method: 'POST',
                headers: { 'content-type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify({ subscriptions }),
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

            resetState();
            close();
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
                    onKeyDown={onFormKeyDown}
                    className="flex min-h-0 min-w-0 flex-col gap-5"
                >
                    <div className="space-y-2">
                        <Label htmlFor="subscription-url" className="sr-only">
                            URL
                        </Label>
                        <div className="flex items-center gap-2">
                            <Input
                                id="subscription-url"
                                type="url"
                                inputMode="url"
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
                                    'Find feeds'
                                )}
                            </Button>
                        </div>
                        {discoverError && (
                            <InputError message={discoverError} />
                        )}
                    </div>

                    {hasCandidates && (
                        <>
                            <h2 id="candidate-list-heading" className="sr-only">
                                Discovered feeds
                            </h2>
                            <div className="-mx-6 min-h-0 flex-1 overflow-y-auto px-6">
                                <FeedCandidateList
                                    aria-labelledby="candidate-list-heading"
                                    candidates={candidates}
                                    existingByFeedUrl={existingByFeedUrl}
                                    selected={selectedMap}
                                    onToggle={toggleCandidate}
                                    onTitleChange={handleTitleChange}
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
                                    `subscriptions.${index}.feed_url`
                                ],
                                fieldErrors[
                                    `subscriptions.${index}.title`
                                ],
                            ].filter(Boolean) as string[];

                            return indexedErrors.length === 0 ? null : (
                                <div
                                    key={item.feed_url}
                                    className="flex flex-col gap-1"
                                >
                                    <p className="text-xs text-muted-foreground">
                                        {item.title || item.feed_url}
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

                    <p className="text-sm font-light text-muted-foreground">
                        Your subscriptions are currently all public. (Selective
                        private subscriptions are coming.)
                    </p>

                    <DialogFooter>
                        <Button
                            type="button"
                            variant="secondary"
                            onClick={close}
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
                                <>
                                    {submitLabel}
                                    <kbd
                                        data-slot="kbd"
                                        aria-hidden="true"
                                        className="ml-1 font-sans text-xs opacity-50"
                                    >
                                        {isMac ? '⌘⏎' : 'Ctrl+⏎'}
                                    </kbd>
                                </>
                            )}
                        </Button>
                    </DialogFooter>
                </form>
            </DialogContent>
        </Dialog>
    );
}
