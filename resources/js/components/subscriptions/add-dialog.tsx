import { Loading03Icon } from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import { useForm } from '@inertiajs/react';
import type { FormEvent, KeyboardEvent } from 'react';
import { useEffect, useRef, useState } from 'react';
import { toast } from 'sonner';

import {
    discover,
    store,
} from '@/actions/App/Http/Controllers/Subscriptions/SubscriptionController';
import InputError from '@/components/input-error';
import { FeedCandidateList } from '@/components/subscriptions/feed-candidate-list';
import type { SelectedMeta } from '@/components/subscriptions/feed-candidate-list';
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

type FeedCandidate = App.Data.Feeds.ResolvedFeedData;
type ExistingSubscription = App.Data.Subscriptions.ExistingSubscriptionData;
type SourceType = App.Enums.SourceType;

type SubscriptionItem = {
    feed_url: string;
    title: string;
    site_url: string;
    source_type: SourceType;
};

type FormShape = {
    subscriptions: SubscriptionItem[];
};

type Props = {
    open: boolean;
    onOpenChange: (open: boolean) => void;
};

const EMPTY_FORM: FormShape = { subscriptions: [] };

function readCsrfToken(): string {
    const match = document.cookie.match(/XSRF-TOKEN=([^;]+)/);

    return match ? decodeURIComponent(match[1]) : '';
}

function toItem(candidate: FeedCandidate): SubscriptionItem {
    return {
        feed_url: candidate.feed_url,
        title: candidate.title ?? '',
        site_url: candidate.site_url ?? '',
        source_type: candidate.source_type,
    };
}

export function AddSubscriptionDialog({ open, onOpenChange }: Props) {
    const [url, setUrl] = useState('');
    const [candidates, setCandidates] = useState<FeedCandidate[] | null>(null);
    const [existingSubscriptions, setExistingSubscriptions] = useState<
        ExistingSubscription[]
    >([]);
    const [discovering, setDiscovering] = useState(false);
    const [discoverError, setDiscoverError] = useState<string | null>(null);
    const candidateListRef = useRef<HTMLDivElement>(null);
    const previousCandidatesRef = useRef<FeedCandidate[] | null>(null);

    const { data, setData, post, processing, errors, reset, clearErrors } =
        useForm<FormShape>(EMPTY_FORM);

    const close = () => {
        onOpenChange(false);
    };

    const handleOpenChangeComplete = (nextOpen: boolean) => {
        if (nextOpen) {
            return;
        }

        reset();
        clearErrors();
        setUrl('');
        setCandidates(null);
        setExistingSubscriptions([]);
        setDiscoverError(null);
        setDiscovering(false);
    };

    const onUrlChange = (next: string) => {
        setUrl(next);

        if (candidates !== null) {
            setCandidates(null);
            setExistingSubscriptions([]);
            setDiscoverError(null);
            setData(EMPTY_FORM);
        }
    };

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

        if (target instanceof HTMLInputElement) {
            event.preventDefault();
        }
    };

    useEffect(() => {
        const previous = previousCandidatesRef.current;
        previousCandidatesRef.current = candidates;

        if (previous !== null || candidates === null) {
            return;
        }

        const container = candidateListRef.current;

        if (!container) {
            return;
        }

        if (data.subscriptions.length === 1) {
            const titleInput =
                container.querySelector<HTMLInputElement>('input[type="text"]');

            if (titleInput) {
                titleInput.focus();
                titleInput.select();

                return;
            }
        }

        const checkbox = container.querySelector<HTMLInputElement>(
            'input[type="checkbox"]:not(:disabled)',
        );
        checkbox?.focus();
    }, [candidates, data.subscriptions.length]);

    const findFeeds = async () => {
        setDiscovering(true);
        setDiscoverError(null);

        try {
            const response = await fetch(discover().url, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    Accept: 'application/json',
                    'X-XSRF-TOKEN': readCsrfToken(),
                    'X-Requested-With': 'XMLHttpRequest',
                },
                credentials: 'same-origin',
                body: JSON.stringify({ url }),
            });

            if (response.status === 422) {
                const body = (await response.json()) as {
                    errors?: { url?: string[] };
                };
                setDiscoverError(
                    body.errors?.url?.[0] ?? 'That URL didn’t resolve.',
                );

                return;
            }

            if (!response.ok) {
                setDiscoverError(
                    'Something went wrong reaching that URL. Try again?',
                );

                return;
            }

            const body = (await response.json()) as {
                candidates: FeedCandidate[];
                existing_subscriptions: ExistingSubscription[];
            };
            setCandidates(body.candidates);
            setExistingSubscriptions(body.existing_subscriptions);

            const existing = new Set(
                body.existing_subscriptions.map((s) => s.feed_url),
            );
            const fresh = body.candidates.filter(
                (c) => !existing.has(c.feed_url),
            );

            if (fresh.length === 1) {
                setData({ subscriptions: [toItem(fresh[0])] });
            }
        } finally {
            setDiscovering(false);
        }
    };

    const hasCandidates = candidates !== null && candidates.length > 0;
    const selectedCount = data.subscriptions.length;
    const submitDisabled = processing || selectedCount === 0;

    const selectedMap: Record<string, SelectedMeta> = Object.fromEntries(
        data.subscriptions.map((item) => [
            item.feed_url,
            { title: item.title, source_type: item.source_type },
        ]),
    );

    const existingByFeedUrl: Record<string, string | null> = Object.fromEntries(
        existingSubscriptions.map((s) => [s.feed_url, s.title]),
    );

    const toggleCandidate = (candidate: FeedCandidate) => {
        const exists = data.subscriptions.some(
            (item) => item.feed_url === candidate.feed_url,
        );

        const next = exists
            ? data.subscriptions.filter(
                  (item) => item.feed_url !== candidate.feed_url,
              )
            : [...data.subscriptions, toItem(candidate)];

        setData('subscriptions', next);
    };

    const updateItem = (feedUrl: string, patch: Partial<SubscriptionItem>) => {
        setData(
            'subscriptions',
            data.subscriptions.map((item) =>
                item.feed_url === feedUrl ? { ...item, ...patch } : item,
            ),
        );
    };

    const submit = (event: FormEvent) => {
        event.preventDefault();

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

        post(store().url, {
            preserveScroll: true,
            onSuccess: () => {
                toast.success(
                    selectedCount === 1
                        ? 'Source added.'
                        : `${selectedCount} sources added.`,
                );
                close();
            },
        });
    };

    const submitLabel =
        selectedCount > 1 ? `Add ${selectedCount} sources` : 'Add source';

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
                                aria-invalid={discoverError ? true : undefined}
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
                        <div className="-mx-6 min-h-0 flex-1 overflow-y-auto px-6">
                            <FeedCandidateList
                                containerRef={candidateListRef}
                                candidates={candidates}
                                existingByFeedUrl={existingByFeedUrl}
                                selected={selectedMap}
                                onToggle={toggleCandidate}
                                onTitleChange={(feedUrl, title) =>
                                    updateItem(feedUrl, { title })
                                }
                                onSourceTypeChange={(feedUrl, type) =>
                                    updateItem(feedUrl, { source_type: type })
                                }
                            />
                        </div>
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
                                errors[
                                    `subscriptions.${index}.source_type` as keyof typeof errors
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

                    <p className="font-handwritten text-sm text-muted-foreground">
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
                                submitLabel
                            )}
                        </Button>
                    </DialogFooter>
                </form>
            </DialogContent>
        </Dialog>
    );
}
