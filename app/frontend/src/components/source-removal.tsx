import { Dialog } from '@base-ui/react/dialog';
import { useRef, useState, type ReactNode, type RefObject } from 'react';
import { toast } from 'sonner';

import { Button } from '@/components/ui/button';
import { DialogBackdrop, DialogPopup } from '@/components/ui/dialog';
import { Link } from '@/components/ui/link';
import { Frame } from '@/design/frame';
import { Text } from '@/design/text';
import { ApiError, classifyMutationError, describeMutationError } from '@/lib/api';
import { ExternalLinkIcon } from '@/components/icons';
import { entryHref, newsletterSourceHref, PATHS } from '@/lib/paths';
import {
    deleteNewsletter,
    DELETION,
    GONE,
    isGone,
    REMOVAL,
    removeSource,
    unsubscribeNewsletter,
    type EditableSource,
} from '@/lib/source-edit';

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
            setError({
                message: describeMutationError(failure, fallback),
                reauth: classifyMutationError(failure) === 'reauth',
            });
        } finally {
            setPending(false);
        }
    }
    return { pending, error, run };
}

type RemovalCopy = { title: (name: string) => string; description: string; action: string; done: string };

function RemovalDialog({
    copy,
    failed,
    confirm,
    onRemoved,
    removedFocus,
    children,
    ...props
}: DialogProps & {
    copy: RemovalCopy;
    children?: ReactNode;
    failed: string;
    confirm: () => Promise<unknown>;
    onRemoved: () => void;
    removedFocus?: RefObject<HTMLElement | null>;
}) {
    const { source, open, onClose } = props;
    const removed = useRef(false);
    const mutation = useMutation(
        props,
        () => {
            removed.current = true;
            toast.success(copy.done);
            onRemoved();
        },
        failed,
    );

    return (
        <SourceDialog
            description={copy.description}
            finalFocus={removedFocus ? () => (removed.current ? removedFocus.current : true) : undefined}
            onClose={onClose}
            open={open}
            pending={mutation.pending}
            title={copy.title(source.title)}
        >
            {children || mutation.error ? (
                <Frame gap="m">
                    {children}
                    <ErrorLine error={mutation.error} />
                </Frame>
            ) : null}
            <DialogActions
                destructive
                label={copy.action}
                onCancel={onClose}
                onConfirm={() => void mutation.run(confirm)}
                pending={mutation.pending}
            />
        </SourceDialog>
    );
}

export function RemoveDialog(
    props: DialogProps & {
        onRemoved: () => void;
        removedFocus?: RefObject<HTMLElement | null>;
    },
) {
    const copy = REMOVAL[props.source.kind];
    return <RemovalDialog {...props} confirm={() => removeSource(props.source)} copy={copy} failed={copy.failed} />;
}

function unsubscribeRefused(error: unknown): boolean {
    return error instanceof ApiError && (error.status === 502 || error.status === 422);
}

export function DeleteNewsletterDialog(
    props: Omit<DialogProps, 'source'> & {
        source: Extract<EditableSource, { kind: 'newsletter' }>;
        onRemoved: () => void;
        removedFocus?: RefObject<HTMLElement | null>;
    },
) {
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
        <RemovalDialog
            {...props}
            confirm={confirm}
            copy={DELETION[manual ? 'manual' : 'automatic']}
            failed="Couldn’t delete this newsletter."
        >
            {manual && source.latestEntrySlug ? (
                <Text>
                    <Text as="span" size="sm">
                        <Link
                            href={entryHref(source.latestEntrySlug, newsletterSourceHref(source.id))}
                            rel="noopener noreferrer"
                            target="_blank"
                        >
                            Open latest issue in a new tab
                            <ExternalLinkIcon aria-hidden="true" size="sm" />
                        </Link>
                    </Text>
                </Text>
            ) : null}
        </RemovalDialog>
    );
}

function SourceDialog({
    title,
    description,
    initialFocus,
    finalFocus,
    open,
    pending,
    onClose,
    children,
}: {
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
                <DialogPopup finalFocus={finalFocus} initialFocus={initialFocus} size="md">
                    <Frame gap="2xl">
                        <Frame gap="2xs">
                            <Dialog.Title render={<Text as="h2" break="words" size="lg" weight="medium" />}>
                                {title}
                            </Dialog.Title>
                            {description ? (
                                <Dialog.Description render={<Text size="sm" tone="muted" />}>
                                    {description}
                                </Dialog.Description>
                            ) : null}
                        </Frame>
                        {children}
                    </Frame>
                </DialogPopup>
            </Dialog.Portal>
        </Dialog.Root>
    );
}

function ErrorLine({ error }: { error: MutationError | null }) {
    if (!error) return null;
    return (
        <Text role="alert" size="sm" tone="destructive">
            {error.message} {error.reauth ? <Link href={PATHS.login}>Sign in again</Link> : null}
        </Text>
    );
}

function DialogActions({
    label,
    pending,
    disabled = false,
    destructive = false,
    onCancel,
    onConfirm,
}: {
    label: string;
    pending: boolean;
    disabled?: boolean;
    destructive?: boolean;
    onCancel: () => void;
    onConfirm?: () => void;
}) {
    return (
        <Frame direction="row" gap="s" justify="end">
            <Button disabled={pending} onClick={onCancel} type="button" variant="ghost">
                Cancel
            </Button>
            <Button
                disabled={pending || disabled}
                onClick={onConfirm}
                type={onConfirm ? 'button' : 'submit'}
                variant={destructive ? 'destructive' : 'default'}
            >
                {label}
            </Button>
        </Frame>
    );
}
