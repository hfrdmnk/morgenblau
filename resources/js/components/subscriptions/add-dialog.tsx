import { Loading03Icon } from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import { useForm, useHttp } from '@inertiajs/react';
import type { FormEvent, KeyboardEvent } from 'react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import {
    discover,
    store,
} from '@/actions/App/Http/Controllers/Subscriptions/SubscriptionController';
import InputError from '@/components/input-error';
import { FeedCandidateList } from '@/components/subscriptions/feed-candidate-list';
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

type FeedCandidate = App.Data.Feeds.ResolvedFeedData;
type ExistingSubscription = App.Data.Subscriptions.ExistingSubscriptionData;
type DiscoverResult = App.Data.Subscriptions.DiscoverResultData;

type SubscriptionItem = {
    feed_url: string;
    title: string;
    site_url: string;
};

type FormShape = {
    subscriptions: SubscriptionItem[];
};

type Props = {
    open: boolean;
    onOpenChange: (open: boolean) => void;
};

const EMPTY_FORM: FormShape = { subscriptions: [] };

function toItem(candidate: FeedCandidate): SubscriptionItem {
    return {
        feed_url: candidate.feed_url,
        title: candidate.title ?? '',
        site_url: candidate.site_url ?? '',
    };
}

export function AddSubscriptionDialog({ open, onOpenChange }: Props) {
    const [candidates, setCandidates] = useState<FeedCandidate[] | null>(null);
    const [existingSubscriptions, setExistingSubscriptions] = useState<
        ExistingSubscription[]
    >([]);
    const firstCheckboxRef = useRef<HTMLInputElement>(null);
    const firstTitleInputRef = useRef<HTMLInputElement>(null);
    const previousCandidatesRef = useRef<FeedCandidate[] | null>(null);

    const { data, setData, post, processing, errors, reset, clearErrors } =
        useForm<FormShape>(EMPTY_FORM);

    const discoverHttp = useHttp<{ url: string }, DiscoverResult>(
        'post',
        discover().url,
        { url: '' },
    );

    const isMac = useMemo(() => isMacPlatform(), []);

    const close = () => {
        onOpenChange(false);
    };

    const resetState = () => {
        reset();
        clearErrors();
        discoverHttp.cancel();
        discoverHttp.setData('url', '');
        discoverHttp.clearErrors();
        setCandidates(null);
        setExistingSubscriptions([]);
    };

    const handleOpenChangeComplete = (nextOpen: boolean) => {
        if (nextOpen) {
            return;
        }

        resetState();
    };

    const onUrlChange = (next: string) => {
        discoverHttp.setData('url', next);

        if (candidates !== null) {
            setCandidates(null);
            setExistingSubscriptions([]);
            discoverHttp.clearErrors();
            setData(EMPTY_FORM);
        }
    };

    const findFeeds = () => {
        if (!discoverHttp.data.url.trim()) {
            return;
        }

        discoverHttp.cancel();
        discoverHttp.post(discover().url, {
            onSuccess: (response) => {
                setCandidates(response.candidates);
                setExistingSubscriptions(response.existing_subscriptions);

                const existing = new Set(
                    response.existing_subscriptions.map((s) => s.feed_url),
                );
                const fresh = response.candidates.filter(
                    (c) => !existing.has(c.feed_url),
                );

                if (fresh.length === 1) {
                    setData({ subscriptions: [toItem(fresh[0])] });
                }
            },
            onHttpException: (response) => {
                if (response.status >= 500) {
                    discoverHttp.setError(
                        'url',
                        'Couldn’t reach that URL. Try again?',
                    );

                    return true;
                }
            },
        });
    };

    const onUrlKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
        if (event.key !== 'Enter' || event.nativeEvent.isComposing) {
            return;
        }

        if (
            candidates !== null ||
            !discoverHttp.data.url.trim() ||
            discoverHttp.processing
        ) {
            return;
        }

        event.preventDefault();
        findFeeds();
    };

    const hasCandidates = candidates !== null && candidates.length > 0;
    const selectedCount = data.subscriptions.length;
    const submitDisabled = processing || selectedCount === 0;

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

            if (hasCandidates && selectedCount > 0 && !processing) {
                event.currentTarget.requestSubmit();
            }

            return;
        }

        // Swallow Enter on the discovery URL input (it has its own handler) and
        // on checkboxes; let it bubble inside title inputs so the form submits
        // naturally for keyboard / screen-reader users.
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

        if (data.subscriptions.length === 1) {
            const titleInput = firstTitleInputRef.current;

            if (titleInput) {
                titleInput.focus();
                titleInput.select();

                return;
            }
        }

        firstCheckboxRef.current?.focus();
    }, [candidates, data.subscriptions.length]);

    const selectedMap = useMemo(
        () =>
            Object.fromEntries(
                data.subscriptions.map((item) => [
                    item.feed_url,
                    { title: item.title },
                ]),
            ),
        [data.subscriptions],
    );

    const existingByFeedUrl: Record<string, string | null> = useMemo(
        () =>
            Object.fromEntries(
                existingSubscriptions.map((s) => [s.feed_url, s.title]),
            ),
        [existingSubscriptions],
    );

    const toggleCandidate = useCallback(
        (candidate: FeedCandidate) => {
            setData((current) => {
                const exists = current.subscriptions.some(
                    (item) => item.feed_url === candidate.feed_url,
                );

                return {
                    ...current,
                    subscriptions: exists
                        ? current.subscriptions.filter(
                              (item) => item.feed_url !== candidate.feed_url,
                          )
                        : [...current.subscriptions, toItem(candidate)],
                };
            });
        },
        [setData],
    );

    const handleTitleChange = useCallback(
        (feedUrl: string, title: string) => {
            setData((current) => ({
                ...current,
                subscriptions: current.subscriptions.map((item) =>
                    item.feed_url === feedUrl ? { ...item, title } : item,
                ),
            }));
        },
        [setData],
    );

    const submit = (event: FormEvent) => {
        event.preventDefault();

        if (processing) {
            return;
        }

        if (!hasCandidates) {
            if (!discoverHttp.data.url.trim() || discoverHttp.processing) {
                return;
            }

            findFeeds();

            return;
        }

        if (selectedCount === 0) {
            return;
        }

        post(store().url, {
            preserveScroll: true,
            onSuccess: () => {
                resetState();
                close();
            },
        });
    };

    const submitLabel =
        selectedCount > 1 ? `Add ${selectedCount} sources` : 'Add source';

    const topLevelError =
        (errors as Record<string, string | undefined>).subscriptions ??
        (errors as Record<string, string | undefined>).message;

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
                                value={discoverHttp.data.url}
                                onChange={(event) =>
                                    onUrlChange(event.target.value)
                                }
                                onKeyDown={onUrlKeyDown}
                                aria-invalid={
                                    discoverHttp.errors.url ? true : undefined
                                }
                                className="flex-1"
                            />
                            <Button
                                type="button"
                                variant="secondary"
                                onClick={findFeeds}
                                disabled={
                                    discoverHttp.processing ||
                                    !discoverHttp.data.url.trim() ||
                                    candidates !== null
                                }
                            >
                                {discoverHttp.processing ? (
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
                        {discoverHttp.errors.url && (
                            <InputError message={discoverHttp.errors.url} />
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
                        data.subscriptions.map((item, index) => {
                            const indexedErrors = [
                                errors[
                                    `subscriptions.${index}.feed_url` as keyof typeof errors
                                ],
                                errors[
                                    `subscriptions.${index}.title` as keyof typeof errors
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
                            disabled={processing}
                        >
                            Cancel
                        </Button>
                        <Button type="submit" disabled={submitDisabled}>
                            {processing ? (
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
