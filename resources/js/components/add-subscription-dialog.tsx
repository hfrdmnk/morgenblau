import { Loading03Icon } from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import { useForm } from '@inertiajs/react';
import type { FormEvent } from 'react';
import { useState } from 'react';
import { toast } from 'sonner';

import {
    discover,
    store,
} from '@/actions/App/Http/Controllers/SubscriptionController';
import type {
    FeedCandidate,
    SelectedMeta,
    SourceType,
} from '@/components/feed-candidate-list';
import { FeedCandidateList } from '@/components/feed-candidate-list';
import InputError from '@/components/input-error';
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
    const [existingFeedUrls, setExistingFeedUrls] = useState<string[]>([]);
    const [discovering, setDiscovering] = useState(false);
    const [discoverError, setDiscoverError] = useState<string | null>(null);

    const { data, setData, post, processing, errors, reset, clearErrors } =
        useForm<FormShape>(EMPTY_FORM);

    const close = () => {
        reset();
        clearErrors();
        setUrl('');
        setCandidates(null);
        setExistingFeedUrls([]);
        setDiscoverError(null);
        setDiscovering(false);
        onOpenChange(false);
    };

    const handleOpenChange = (next: boolean) => {
        if (!next) {
            close();

            return;
        }

        onOpenChange(next);
    };

    const onUrlChange = (next: string) => {
        setUrl(next);

        if (candidates !== null) {
            setCandidates(null);
            setExistingFeedUrls([]);
            setDiscoverError(null);
            setData(EMPTY_FORM);
        }
    };

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
                existing_feed_urls: string[];
            };
            setCandidates(body.candidates);
            setExistingFeedUrls(body.existing_feed_urls);

            const existing = new Set(body.existing_feed_urls);
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
        <Dialog open={open} onOpenChange={handleOpenChange}>
            <DialogContent>
                <DialogHeader>
                    <DialogTitle>Add a source</DialogTitle>
                    <DialogDescription>
                        Paste a website, RSS feed, YouTube channel, or podcast
                        URL.
                    </DialogDescription>
                </DialogHeader>

                <form onSubmit={submit} className="flex min-w-0 flex-col gap-5">
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
                        <FeedCandidateList
                            candidates={candidates}
                            existingFeedUrls={existingFeedUrls}
                            selected={selectedMap}
                            onToggle={toggleCandidate}
                            onTitleChange={(feedUrl, title) =>
                                updateItem(feedUrl, { title })
                            }
                            onSourceTypeChange={(feedUrl, type) =>
                                updateItem(feedUrl, { source_type: type })
                            }
                        />
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
