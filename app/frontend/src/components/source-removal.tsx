import { Dialog } from '@base-ui/react/dialog';
import { useRef, useState, type ReactNode, type RefObject } from 'react';
import { toast } from 'sonner';

import { Button } from '@/components/ui/button';
import { DialogBackdrop, DialogPopup } from '@/components/ui/dialog';
import { Link } from '@/components/ui/link';
import { ApiError, classifyMutationError, describeMutationError } from '@/lib/api';
import { ExternalLinkIcon } from '@/components/icons';
import { entryHref, newsletterSourceHref, PATHS } from '@/lib/paths';
import { deleteNewsletter, GONE, isGone, removeSource, unsubscribeNewsletter, type EditableSource } from '@/lib/source-edit';

// onChanged refreshes the page's copy; a source that vanished closes its dialog so the refresh can drop it.
type DialogProps = { source: EditableSource; open: boolean; onClose: () => void; onChanged: () => void };
type MutationError = { message: string; reauth: boolean };

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

const REMOVAL = {
    feed: {
        title: (name: string) => `Delete “${name}”?`,
        description: 'Its posts leave your digest. Posts you saved stay in your Library.',
        action: 'Delete source',
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

type RemovalCopy = { title: (name: string) => string; description: string; action: string; done: string };

function RemovalDialog({ copy, failed, confirm, onRemoved, removedFocus, children, ...props }: DialogProps & {
    copy: RemovalCopy;
    children?: ReactNode;
    failed: string;
    confirm: () => Promise<unknown>;
    onRemoved: () => void;
    removedFocus?: RefObject<HTMLElement | null>;
}) {
    const { source, open, onClose } = props;
    const removed = useRef(false);
    const mutation = useMutation(props, () => {
        removed.current = true;
        toast.success(copy.done);
        onRemoved();
    }, failed);

    return (
        <SourceDialog
            description={copy.description}
            finalFocus={removedFocus ? () => (removed.current ? removedFocus.current : true) : undefined}
            onClose={onClose}
            open={open}
            pending={mutation.pending}
            title={copy.title(source.title)}
        >
            {children}
            <ErrorLine error={mutation.error} />
            <DialogActions destructive label={copy.action} onCancel={onClose} onConfirm={() => void mutation.run(confirm)} pending={mutation.pending} />
        </SourceDialog>
    );
}

export function RemoveDialog(props: DialogProps & {
    onRemoved: () => void;
    removedFocus?: RefObject<HTMLElement | null>;
}) {
    const copy = REMOVAL[props.source.kind];
    return <RemovalDialog {...props} confirm={() => removeSource(props.source)} copy={copy} failed={copy.failed} />;
}

const DELETION = {
    automatic: {
        title: (name: string) => `Unsubscribe from “${name}”?`,
        description: 'Morgenblau asks the sender to remove your address, then deletes this newsletter and its unsaved issues. Saved issues stay in your Library.',
        action: 'Unsubscribe and delete',
        done: 'Unsubscribed and deleted',
    },
    manual: {
        title: (name: string) => `Delete “${name}”?`,
        description: 'Unsubscribe manually, usually through the link at the bottom of an issue, then delete it here. Saved issues stay in your Library. If you don’t unsubscribe, its next issue shows up again as a new newsletter.',
        action: 'Delete newsletter',
        done: 'Newsletter deleted',
    },
};

// The sender refused or never offered one-click unsubscribe; nothing changed, so the reader can still delete by hand.
function unsubscribeRefused(error: unknown): boolean {
    return error instanceof ApiError && (error.status === 502 || error.status === 422);
}

export function DeleteNewsletterDialog(props: Omit<DialogProps, 'source'> & {
    source: Extract<EditableSource, { kind: 'newsletter' }>;
    onRemoved: () => void;
    removedFocus?: RefObject<HTMLElement | null>;
}) {
    const { source } = props;
    const [manual, setManual] = useState(!source.canUnsubscribe);

    async function confirm() {
        if (manual) return deleteNewsletter(source);
        try {
            return await unsubscribeNewsletter(source);
        } catch (error) {
            if (unsubscribeRefused(error)) setManual(true);
            throw error;
        }
    }

    return (
        <RemovalDialog {...props} confirm={confirm} copy={DELETION[manual ? 'manual' : 'automatic']} failed="Couldn’t delete this newsletter.">
            {manual && source.latestEntrySlug ? (
                <Link className="text-sm" href={entryHref(source.latestEntrySlug, newsletterSourceHref(source.id))} rel="noopener noreferrer" target="_blank">
                    Open latest issue in a new tab
                    <ExternalLinkIcon aria-hidden="true" className="size-3.5" />
                </Link>
            ) : null}
        </RemovalDialog>
    );
}

function SourceDialog({ title, description, initialFocus, finalFocus, open, pending, onClose, children }: {
    title: string;
    description?: string;
    initialFocus?: RefObject<HTMLElement | null>;
    finalFocus?: () => HTMLElement | null | boolean;
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
                <DialogBackdrop />
                <DialogPopup finalFocus={finalFocus} initialFocus={initialFocus} className="max-w-md">
                    <Dialog.Title className="pr-8 text-lg font-medium break-words">{title}</Dialog.Title>
                    {description ? <Dialog.Description className="mt-1 text-sm text-muted-foreground">{description}</Dialog.Description> : null}
                    <div className="mt-6">{children}</div>
                </DialogPopup>
            </Dialog.Portal>
        </Dialog.Root>
    );
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
