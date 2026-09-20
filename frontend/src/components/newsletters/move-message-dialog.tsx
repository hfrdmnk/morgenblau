import { MailIcon, SpinnerIcon } from '@proicons/react';
import { useEffect, useState, type FormEvent } from 'react';

import { InputError } from '@/components/input-error';
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
import { api } from '@/lib/api';
import {
    emitNewsletterMutation,
    fetchNewsletters,
    type NewsletterSource,
} from '@/lib/newsletters';
import { cn } from '@/lib/utils';

type Props = {
    open: boolean;
    onOpenChange: (open: boolean) => void;
    messageId: string;
    currentSourceId: string | undefined;
    onMoved: (source: NewsletterSource) => void;
};

type DestinationChoice = {
    sources: NewsletterSource[];
    loading: boolean;
    selectedId: string | null;
    creating: boolean;
    error?: string;
};

const emptyChoice: DestinationChoice = {
    sources: [],
    loading: true,
    selectedId: null,
    creating: false,
};

async function loadDestinations(
    currentSourceId: string | undefined,
    signal: AbortSignal,
): Promise<DestinationChoice> {
    try {
        const { active } = await fetchNewsletters(signal);
        const sources = active.filter(
            (source) => source.id !== currentSourceId,
        );
        return {
            sources,
            loading: false,
            selectedId: sources[0]?.id ?? null,
            creating: sources.length === 0,
        };
    } catch {
        return {
            ...emptyChoice,
            loading: false,
            error: "Couldn't load your newsletters. Try again.",
        };
    }
}

function useDestinationChoice(open: boolean, currentSourceId: string | undefined) {
    const [choice, setChoice] = useState<DestinationChoice>(emptyChoice);

    useEffect(() => {
        if (!open) return;
        const abort = new AbortController();
        void loadDestinations(currentSourceId, abort.signal).then((nextChoice) => {
            if (!abort.signal.aborted) setChoice(nextChoice);
        });
        return () => abort.abort();
    }, [open, currentSourceId]);

    return {
        ...choice,
        selectSource: (selectedId: string) =>
            setChoice((current) => ({
                ...current,
                selectedId,
                creating: false,
            })),
        createSource: () =>
            setChoice((current) => ({ ...current, creating: true })),
        reset: () => setChoice(emptyChoice),
    };
}

function buildMovePayload(
    creating: boolean,
    selectedId: string | null,
    newTitle: string,
) {
    if (creating) {
        const title = newTitle.trim();
        return title ? { newSourceTitle: title } : null;
    }
    return selectedId ? { sourceId: selectedId } : null;
}

function DestinationButton({
    source,
    selected,
    onSelect,
}: {
    source: NewsletterSource;
    selected: boolean;
    onSelect: (id: string) => void;
}) {
    return (
        <button
            type="button"
            aria-pressed={selected}
            onClick={() => onSelect(source.id)}
            className={cn(
                'flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left outline-none transition-colors focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring focus-visible:outline-solid',
                selected ? 'bg-accent' : 'hover:bg-muted',
            )}
        >
            <MailIcon className="size-4 text-muted-foreground" />
            <span className="min-w-0 flex-1 truncate text-body">
                {source.title}
            </span>
        </button>
    );
}

function DestinationOptions({
    sources,
    loading,
    selectedId,
    creating,
    onSelect,
    onCreate,
}: {
    sources: NewsletterSource[];
    loading: boolean;
    selectedId: string | null;
    creating: boolean;
    onSelect: (id: string) => void;
    onCreate: () => void;
}) {
    if (loading) {
        return (
            <p className="flex items-center gap-2 text-label text-muted-foreground">
                <SpinnerIcon className="size-4 motion-safe:animate-spin" />
                Loading newsletters…
            </p>
        );
    }

    return (
        <div className="max-h-56 space-y-1 overflow-y-auto">
            {sources.map((source) => (
                <DestinationButton
                    key={source.id}
                    source={source}
                    selected={!creating && selectedId === source.id}
                    onSelect={onSelect}
                />
            ))}
            <button
                type="button"
                aria-pressed={creating}
                onClick={onCreate}
                className={cn(
                    'w-full rounded-xl px-3 py-2 text-left text-body outline-none transition-colors focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring focus-visible:outline-solid',
                    creating ? 'bg-accent' : 'hover:bg-muted',
                )}
            >
                New newsletter…
            </button>
        </div>
    );
}

function NewNewsletterTitle({
    creating,
    title,
    onChange,
}: {
    creating: boolean;
    title: string;
    onChange: (title: string) => void;
}) {
    if (!creating) return null;
    return (
        <div className="space-y-2">
            <Label htmlFor="new-newsletter-title">Title</Label>
            <Input
                id="new-newsletter-title"
                autoFocus
                value={title}
                onChange={(event) => onChange(event.target.value)}
                placeholder="Newsletter title"
            />
        </div>
    );
}

function MoveButtonLabel({ submitting }: { submitting: boolean }) {
    if (!submitting) return 'Move issue';
    return (
        <>
            <SpinnerIcon className="size-4 motion-safe:animate-spin" />
            Moving…
        </>
    );
}

function MoveFooter({
    loading,
    submitting,
    canSubmit,
    onCancel,
}: {
    loading: boolean;
    submitting: boolean;
    canSubmit: boolean;
    onCancel: () => void;
}) {
    return (
        <DialogFooter>
            <Button
                type="button"
                variant="secondary"
                onClick={onCancel}
                disabled={submitting}
            >
                Cancel
            </Button>
            <Button type="submit" disabled={loading || submitting || !canSubmit}>
                <MoveButtonLabel submitting={submitting} />
            </Button>
        </DialogFooter>
    );
}

export function MoveMessageDialog({
    open,
    onOpenChange,
    messageId,
    currentSourceId,
    onMoved,
}: Props) {
    const destinations = useDestinationChoice(open, currentSourceId);
    const [newTitle, setNewTitle] = useState('');
    const [submitting, setSubmitting] = useState(false);
    const [moveError, setMoveError] = useState<string>();

    const submit = async (event: FormEvent) => {
        event.preventDefault();
        const payload = buildMovePayload(
            destinations.creating,
            destinations.selectedId,
            newTitle,
        );
        if (!payload) return;
        setSubmitting(true);
        setMoveError(undefined);
        try {
            const response = await api<{ source: NewsletterSource }>(
                `/api/newsletters/messages/${encodeURIComponent(messageId)}/move`,
                { method: 'POST', body: payload },
            );
            onMoved(response.source);
            emitNewsletterMutation();
            onOpenChange(false);
        } catch {
            setMoveError("Couldn't move this issue. Try again.");
        } finally {
            setSubmitting(false);
        }
    };

    const reset = (nextOpen: boolean) => {
        if (nextOpen) return;
        destinations.reset();
        setNewTitle('');
        setMoveError(undefined);
    };

    return (
        <Dialog
            open={open}
            onOpenChange={onOpenChange}
            onOpenChangeComplete={reset}
        >
            <DialogContent>
                <DialogHeader>
                    <DialogTitle>Move this issue</DialogTitle>
                    <DialogDescription>
                        Correct its newsletter without changing how future
                        emails are grouped.
                    </DialogDescription>
                </DialogHeader>
                <form onSubmit={submit} className="flex min-h-0 flex-col gap-4">
                    <DestinationOptions
                        sources={destinations.sources}
                        loading={destinations.loading}
                        selectedId={destinations.selectedId}
                        creating={destinations.creating}
                        onSelect={destinations.selectSource}
                        onCreate={destinations.createSource}
                    />
                    <NewNewsletterTitle
                        creating={destinations.creating}
                        title={newTitle}
                        onChange={setNewTitle}
                    />
                    <InputError message={moveError ?? destinations.error} />
                    <MoveFooter
                        loading={destinations.loading}
                        submitting={submitting}
                        canSubmit={
                            destinations.creating
                                ? newTitle.trim().length > 0
                                : destinations.selectedId !== null
                        }
                        onCancel={() => onOpenChange(false)}
                    />
                </form>
            </DialogContent>
        </Dialog>
    );
}
