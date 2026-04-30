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

type FormShape = {
    feed_url: string;
    title: string;
    site_url: string;
    source_type: SourceType;
};

type Props = {
    open: boolean;
    onOpenChange: (open: boolean) => void;
};

const EMPTY_FORM: FormShape = {
    feed_url: '',
    title: '',
    site_url: '',
    source_type: 'rss',
};

function readCsrfToken(): string {
    const match = document.cookie.match(/XSRF-TOKEN=([^;]+)/);

    return match ? decodeURIComponent(match[1]) : '';
}

export function AddSubscriptionDialog({ open, onOpenChange }: Props) {
    const [url, setUrl] = useState('');
    const [candidates, setCandidates] = useState<FeedCandidate[] | null>(null);
    const [discovering, setDiscovering] = useState(false);
    const [discoverError, setDiscoverError] = useState<string | null>(null);

    const { data, setData, post, processing, errors, reset, clearErrors } =
        useForm<FormShape>(EMPTY_FORM);

    const close = () => {
        reset();
        clearErrors();
        setUrl('');
        setCandidates(null);
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

    const selectCandidate = (candidate: FeedCandidate) => {
        setData({
            feed_url: candidate.feed_url,
            title: candidate.title ?? '',
            site_url: candidate.site_url ?? '',
            source_type: candidate.source_type,
        });
    };

    const onUrlChange = (next: string) => {
        setUrl(next);

        if (candidates !== null) {
            setCandidates(null);
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
            };
            setCandidates(body.candidates);

            if (body.candidates.length === 1) {
                selectCandidate(body.candidates[0]);
            }
        } finally {
            setDiscovering(false);
        }
    };

    const submit = (event: FormEvent) => {
        event.preventDefault();

        post(store().url, {
            preserveScroll: true,
            onSuccess: () => {
                toast.success('Source added.');
                close();
            },
        });
    };

    const hasCandidates = candidates !== null && candidates.length > 0;
    const submitDisabled = processing || !data.feed_url;

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

                <form onSubmit={submit} className="flex flex-col gap-5">
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
                            selectedFeedUrl={data.feed_url || null}
                            onSelect={(feedUrl) => {
                                const next = candidates.find(
                                    (c) => c.feed_url === feedUrl,
                                );

                                if (next) {
                                    selectCandidate(next);
                                }
                            }}
                            title={data.title}
                            onTitleChange={(title) => setData('title', title)}
                            sourceType={data.source_type}
                            onSourceTypeChange={(type) =>
                                setData('source_type', type)
                            }
                        />
                    )}

                    {hasCandidates && (
                        <>
                            <InputError message={errors.feed_url} />
                            <InputError message={errors.title} />
                            <InputError message={errors.source_type} />
                        </>
                    )}

                    <p className="font-handwritten text-xs text-muted-foreground">
                        Your subscriptions are currently public. Private
                        subscriptions are coming.
                    </p>

                    <DialogFooter>
                        <Button
                            type="button"
                            variant="ghost"
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
                                'Add source'
                            )}
                        </Button>
                    </DialogFooter>
                </form>
            </DialogContent>
        </Dialog>
    );
}
