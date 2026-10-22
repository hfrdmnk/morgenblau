import { Dialog } from '@base-ui/react/dialog';
import { useId, useRef, useState, type FormEvent, type ReactNode, type RefObject } from 'react';
import { toast } from 'sonner';

import { CloseIcon, MoreIcon, PauseIcon, PenIcon, PlayIcon, TagIcon, TrashIcon } from '@/components/icons';
import { Button } from '@/components/ui/button';
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuSeparator,
    DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Input } from '@/components/ui/input';
import { Link } from '@/components/ui/link';
import { useSources } from '@/hooks/use-sources';
import { ApiError, classifyMutationError, describeMutationError } from '@/lib/api';
import { PATHS } from '@/lib/paths';
import {
    addTags,
    canAddTags,
    hasOverlongTag,
    MAX_TAG_LENGTH,
    MAX_TAGS,
    MAX_TITLE_LENGTH,
    removeSource,
    renameSource,
    resumeNewsletter,
    retagSource,
    titleTooLong,
    type EditableSource,
} from '@/lib/source-edit';

type DialogKind = 'rename' | 'tags' | 'remove';

export function SourceMenu({ source, onChanged, onRemoved = onChanged }: {
    source: EditableSource;
    onChanged: () => void;
    onRemoved?: () => void;
}) {
    const [dialog, setDialog] = useState<DialogKind | null>(null);
    // A fresh key per opening starts each dialog from the source's current values.
    const [opening, setOpening] = useState(0);
    const stopped = source.kind === 'newsletter' && source.stopped;
    const open = (kind: DialogKind) => {
        setOpening((count) => count + 1);
        setDialog(kind);
    };
    const close = () => setDialog(null);

    async function resume() {
        if (source.kind !== 'newsletter') return;
        try {
            await resumeNewsletter(source);
            toast.success('Newsletter resumed');
            onChanged();
        } catch (error) {
            if (isGone(error)) {
                toast.error(GONE);
                onChanged();
            } else {
                toast.error(describeMutationError(error, 'Couldn’t resume this newsletter.'));
            }
        }
    }

    return (
        <>
            <DropdownMenu>
                <DropdownMenuTrigger
                    render={
                        <Button
                            aria-label={`Actions for ${source.title}`}
                            className="text-subtle-foreground hover:text-foreground"
                            size="icon-sm"
                            variant="ghost"
                        />
                    }
                >
                    <MoreIcon className="size-5" />
                </DropdownMenuTrigger>
                <DropdownMenuContent>
                    <DropdownMenuItem onClick={() => open('rename')}>
                        <PenIcon />
                        Rename
                    </DropdownMenuItem>
                    <DropdownMenuItem onClick={() => open('tags')}>
                        <TagIcon />
                        Edit tags
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    {source.kind === 'feed' ? (
                        <DropdownMenuItem onClick={() => open('remove')} variant="destructive">
                            <TrashIcon />
                            Delete
                        </DropdownMenuItem>
                    ) : stopped ? (
                        <DropdownMenuItem onClick={() => void resume()}>
                            <PlayIcon />
                            Resume newsletter
                        </DropdownMenuItem>
                    ) : (
                        <DropdownMenuItem onClick={() => open('remove')} variant="destructive">
                            <PauseIcon />
                            Stop newsletter
                        </DropdownMenuItem>
                    )}
                </DropdownMenuContent>
            </DropdownMenu>
            <RenameDialog key={`rename-${opening}`} onChanged={onChanged} onClose={close} open={dialog === 'rename'} source={source} />
            <TagsDialog key={`tags-${opening}`} onChanged={onChanged} onClose={close} open={dialog === 'tags'} source={source} />
            <RemoveDialog key={`remove-${opening}`} onChanged={onChanged} onClose={close} onRemoved={onRemoved} open={dialog === 'remove'} source={source} />
        </>
    );
}

// onChanged refreshes the page's copy; a source that vanished closes its dialog so the refresh can drop it.
type DialogProps = { source: EditableSource; open: boolean; onClose: () => void; onChanged: () => void };
type MutationError = { message: string; reauth: boolean };

const GONE = 'This source is no longer in your sources.';

function isGone(error: unknown): boolean {
    return error instanceof ApiError && error.status === 404;
}

function useMutation({ onClose, onChanged }: DialogProps, onDone: () => void, fallback: string) {
    const [pending, setPending] = useState(false);
    const [error, setError] = useState<MutationError | null>(null);
    async function run(write: () => Promise<unknown>) {
        setPending(true);
        setError(null);
        try {
            await write();
            onClose();
            onDone();
        } catch (failure) {
            if (isGone(failure)) {
                onClose();
                toast.error(GONE);
                onChanged();
                return;
            }
            setError({ message: describeMutationError(failure, fallback), reauth: classifyMutationError(failure) === 'reauth' });
        } finally {
            setPending(false);
        }
    }
    return { pending, error, run };
}

function RenameDialog(props: DialogProps) {
    const { source, open, onClose, onChanged } = props;
    const inputId = useId();
    const hintId = useId();
    const [title, setTitle] = useState(source.title);
    const mutation = useMutation(props, onChanged, 'Couldn’t rename this source.');
    const trimmed = title.trim();
    const tooLong = titleTooLong(title);
    const unsaveable = !trimmed || trimmed === source.title || tooLong;

    function submit(event: FormEvent) {
        event.preventDefault();
        if (unsaveable) return;
        void mutation.run(() => renameSource(source, trimmed));
    }

    return (
        <SourceDialog
            onClose={onClose}
            open={open}
            pending={mutation.pending}
            title="Rename source"
        >
            <form onSubmit={submit}>
                <label className="text-sm font-medium" htmlFor={inputId}>Name</label>
                <Input
                    aria-describedby={hintId}
                    aria-invalid={tooLong}
                    autoComplete="off"
                    className="mt-2"
                    id={inputId}
                    onChange={(event) => setTitle(event.target.value)}
                    value={title}
                />
                <LimitHint id={hintId} shown={tooLong}>Names can be up to {MAX_TITLE_LENGTH} characters.</LimitHint>
                <ErrorLine error={mutation.error} />
                <DialogActions disabled={unsaveable} label="Save" onCancel={onClose} pending={mutation.pending} />
            </form>
        </SourceDialog>
    );
}

function TagsDialog(props: DialogProps) {
    const { source, open, onClose, onChanged } = props;
    const inputRef = useRef<HTMLInputElement>(null);
    const [tags, setTags] = useState(source.tags);
    const [draft, setDraft] = useState('');
    const mutation = useMutation(props, onChanged, 'Couldn’t save these tags.');
    const overlong = hasOverlongTag(draft);

    function submit(event: FormEvent) {
        event.preventDefault();
        if (overlong) return;
        void mutation.run(() => retagSource(source, addTags(tags, draft)));
    }

    return (
        <SourceDialog
            description="Tags group your sources. Separate several with commas."
            initialFocus={inputRef}
            onClose={onClose}
            open={open}
            pending={mutation.pending}
            title="Edit tags"
        >
            <form onSubmit={submit}>
                <TagChips onRemove={(tag) => setTags(tags.filter((other) => other !== tag))} tags={tags} />
                <TagInput
                    canAdd={canAddTags(tags, draft)}
                    draft={draft}
                    inputRef={inputRef}
                    onAdd={() => {
                        if (!canAddTags(tags, draft)) return;
                        setTags(addTags(tags, draft));
                        setDraft('');
                    }}
                    onDraft={setDraft}
                    overlong={overlong}
                    tags={tags}
                />
                <ErrorLine error={mutation.error} />
                <DialogActions disabled={overlong} label="Save tags" onCancel={onClose} pending={mutation.pending} />
            </form>
        </SourceDialog>
    );
}

function TagChips({ tags, onRemove }: { tags: string[]; onRemove: (tag: string) => void }) {
    if (!tags.length) return <p className="mb-5 text-sm text-subtle-foreground">No tags yet.</p>;
    return (
        <ul aria-label="Tags" className="mb-5 flex flex-wrap gap-2">
            {tags.map((tag) => (
                <li className="flex items-center gap-1 rounded-full bg-muted py-0.5 pr-1 pl-2.5 text-sm text-muted-foreground" key={tag}>
                    {tag}
                    <button
                        aria-label={`Remove tag ${tag}`}
                        className="flex size-5 cursor-pointer items-center justify-center rounded-full hover:bg-secondary hover:text-foreground"
                        onClick={() => onRemove(tag)}
                        type="button"
                    >
                        <CloseIcon className="size-3.5" />
                    </button>
                </li>
            ))}
        </ul>
    );
}

function TagInput({ tags, draft, canAdd, overlong, inputRef, onDraft, onAdd }: {
    tags: string[];
    draft: string;
    canAdd: boolean;
    overlong: boolean;
    inputRef: RefObject<HTMLInputElement | null>;
    onDraft: (draft: string) => void;
    onAdd: () => void;
}) {
    const inputId = useId();
    const listId = useId();
    const hintId = useId();
    const full = tags.length >= MAX_TAGS;
    return (
        <>
            <label className="text-sm font-medium" htmlFor={inputId}>Add a tag</label>
            <div className="mt-2 flex gap-2">
                <Input
                    aria-describedby={hintId}
                    aria-invalid={overlong}
                    autoComplete="off"
                    className="[&::-webkit-calendar-picker-indicator]:hidden!"
                    disabled={full}
                    id={inputId}
                    list={listId}
                    onChange={(event) => onDraft(event.target.value)}
                    ref={inputRef}
                    onKeyDown={(event) => {
                        if (event.key !== 'Enter') return;
                        event.preventDefault();
                        onAdd();
                    }}
                    placeholder={full ? `Up to ${MAX_TAGS} tags` : 'e.g. design'}
                    value={draft}
                />
                <Button disabled={!canAdd} onClick={onAdd} type="button" variant="secondary">Add</Button>
            </div>
            <LimitHint id={hintId} shown={overlong}>Tags can be up to {MAX_TAG_LENGTH} characters.</LimitHint>
            <TagSuggestions exclude={tags} id={listId} />
        </>
    );
}

function TagSuggestions({ id, exclude }: { id: string; exclude: string[] }) {
    const state = useSources<{ tags: string[] }>('/api/subscriptions/tags');
    const taken = new Set(exclude.map((tag) => tag.toLowerCase()));
    const tags = state.status === 'loaded' ? state.data.tags.filter((tag) => !taken.has(tag.toLowerCase())) : [];
    return <datalist id={id}>{tags.map((tag) => <option key={tag} value={tag} />)}</datalist>;
}

const REMOVAL = {
    feed: {
        title: (name: string) => `Delete “${name}”?`,
        description: 'Its posts leave your digest. Posts you saved stay in your Library.',
        action: 'Delete',
        done: 'Source deleted',
        failed: 'Couldn’t delete this source.',
    },
    newsletter: {
        title: (name: string) => `Stop “${name}”?`,
        description: 'Unsaved issues are deleted and new mail from this sender is discarded. Saved issues stay in your Library, and you can resume it later.',
        action: 'Stop newsletter',
        done: 'Newsletter stopped',
        failed: 'Couldn’t stop this newsletter.',
    },
};

function RemoveDialog({ onRemoved, ...props }: DialogProps & { onRemoved: () => void }) {
    const { source, open, onClose } = props;
    const copy = REMOVAL[source.kind];
    const mutation = useMutation(props, () => {
        toast.success(copy.done);
        onRemoved();
    }, copy.failed);

    return (
        <SourceDialog
            description={copy.description}
            onClose={onClose}
            open={open}
            pending={mutation.pending}
            title={copy.title(source.title)}
        >
            <ErrorLine error={mutation.error} />
            <DialogActions
                destructive
                label={copy.action}
                onCancel={onClose}
                onConfirm={() => void mutation.run(() => removeSource(source))}
                pending={mutation.pending}
            />
        </SourceDialog>
    );
}

function SourceDialog({ title, description, initialFocus, open, pending, onClose, children }: {
    title: string;
    description?: string;
    initialFocus?: RefObject<HTMLElement | null>;
    open: boolean;
    pending: boolean;
    onClose: () => void;
    children: ReactNode;
}) {
    return (
        <Dialog.Root
            onOpenChange={(next, details) => {
                if (pending) details.cancel();
                else if (!next) onClose();
            }}
            open={open}
        >
            <Dialog.Portal>
                <Dialog.Backdrop className="fixed inset-0 z-40 bg-foreground/20 transition-opacity duration-(--motion-duration-overlay) ease-out data-[ending-style]:opacity-0 data-[starting-style]:opacity-0" />
                <Dialog.Popup initialFocus={initialFocus} className="fixed top-[clamp(1rem,12vh,8rem)] left-1/2 z-50 max-h-[calc(92dvh-1rem)] w-[calc(100%-2rem)] max-w-md -translate-x-1/2 overflow-y-auto rounded-xl bg-popover p-6 text-popover-foreground shadow-popover transition-[opacity,scale] duration-(--motion-duration-overlay) ease-out data-[ending-style]:scale-[0.98] data-[ending-style]:opacity-0 data-[starting-style]:scale-[0.98] data-[starting-style]:opacity-0">
                    <Dialog.Title className="pr-8 text-lg font-medium break-words">{title}</Dialog.Title>
                    {description ? <Dialog.Description className="mt-1 text-sm text-muted-foreground">{description}</Dialog.Description> : null}
                    <div className="mt-6">{children}</div>
                </Dialog.Popup>
            </Dialog.Portal>
        </Dialog.Root>
    );
}

function LimitHint({ id, shown, children }: { id: string; shown: boolean; children: ReactNode }) {
    if (!shown) return null;
    return <p className="mt-2 text-sm text-destructive" id={id}>{children}</p>;
}

function ErrorLine({ error }: { error: MutationError | null }) {
    if (!error) return null;
    return (
        <p className="mt-3 text-sm text-destructive" role="alert">
            {error.message}{' '}
            {error.reauth ? <Link href={PATHS.login}>Sign in again</Link> : null}
        </p>
    );
}

function DialogActions({ label, pending, disabled = false, destructive = false, onCancel, onConfirm }: {
    label: string;
    pending: boolean;
    disabled?: boolean;
    destructive?: boolean;
    onCancel: () => void;
    onConfirm?: () => void;
}) {
    return (
        <div className="mt-6 flex justify-end gap-2">
            <Button disabled={pending} onClick={onCancel} type="button" variant="ghost">Cancel</Button>
            <Button
                disabled={pending || disabled}
                onClick={onConfirm}
                type={onConfirm ? 'button' : 'submit'}
                variant={destructive ? 'destructive' : 'default'}
            >
                {label}
            </Button>
        </div>
    );
}
