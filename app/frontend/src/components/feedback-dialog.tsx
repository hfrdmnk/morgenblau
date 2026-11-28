import { Dialog } from '@base-ui/react/dialog';
import { AnimatePresence, useReducedMotion } from 'motion/react';
import { useEffect, useId, useRef, useState, type ClipboardEvent, type DragEvent } from 'react';
import { toast } from 'sonner';

import { ArrowUpIcon, FeedbackIcon, ImageIcon, LoadingIcon } from '@/components/icons';
import { Button } from '@/components/ui/button';
import {
    AttachmentThumb,
    Composer,
    ComposerDialog,
    ComposerDropOverlay,
    ComposerField,
    ComposerHint,
    ComposerStatus,
} from '@/components/ui/composer';
import { Kbd } from '@/components/ui/kbd';
import { Frame } from '@/design/frame';
import { MotionFrame } from '@/design/motion';
import { Text } from '@/design/text';
import { describeMutationError } from '@/lib/api';
import { FEEDBACK_IMAGE_TYPES, MAX_FEEDBACK_IMAGE_BYTES, MAX_FEEDBACK_IMAGES, sendFeedback } from '@/lib/feedback';

const THUMB = { duration: 0.16, ease: [0.16, 1, 0.3, 1] as const };
const SEND_SHORTCUT = /Mac|iPhone|iPad/.test(navigator.platform) ? '⌘ Enter' : 'Ctrl Enter';

type Attachment = { id: string; file: File; url: string };

export function FeedbackDialog() {
    const [open, setOpen] = useState(false);
    const [message, setMessage] = useState('');
    const [attachments, setAttachments] = useState<Attachment[]>([]);
    const [notice, setNotice] = useState<string | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [sending, setSending] = useState(false);
    const textareaRef = useRef<HTMLTextAreaElement>(null);
    const fieldId = useId();
    const { dragging, intakeHandlers } = useImageIntake(add);
    const canSend = message.trim() !== '' && !sending;

    const urls = useRef(new Set<string>());
    useEffect(() => {
        const live = urls.current;
        return () => live.forEach((url) => URL.revokeObjectURL(url));
    }, []);

    function add(files: Iterable<File>) {
        const incoming = [...files];
        if (incoming.length === 0) return;
        const { accepted, refused } = acceptImages(attachments, incoming);
        const added = accepted.map((file) => {
            const url = URL.createObjectURL(file);
            urls.current.add(url);
            return { id: crypto.randomUUID(), file, url };
        });
        setAttachments([...attachments, ...added]);
        setNotice(refused);
    }

    function remove(gone: Attachment) {
        URL.revokeObjectURL(gone.url);
        urls.current.delete(gone.url);
        setAttachments(attachments.filter((a) => a.id !== gone.id));
        setNotice(null);
    }

    async function send() {
        if (!canSend) return;
        setSending(true);
        setError(null);
        try {
            await sendFeedback(
                message.trim(),
                attachments.map((a) => a.file),
            );
            urls.current.forEach((url) => URL.revokeObjectURL(url));
            urls.current.clear();
            setAttachments([]);
            setMessage('');
            setNotice(null);
            setOpen(false);
            toast.success('Feedback sent. Thank you.');
        } catch (failure) {
            setError(
                describeMutationError(failure, 'Couldn’t send your feedback. Check your connection and try again.'),
            );
        } finally {
            setSending(false);
        }
    }

    return (
        <Dialog.Root
            open={open}
            onOpenChange={(next, details) => {
                if (sending) details.cancel();
                else setOpen(next);
            }}
        >
            <Dialog.Trigger
                render={
                    <Button aria-label="Send feedback" size="icon-sm" variant="subtle">
                        <FeedbackIcon size="xl" />
                    </Button>
                }
            />
            <ComposerDialog initialFocus={textareaRef} title="Send feedback" {...intakeHandlers}>
                <Composer
                    dragging={dragging}
                    onSubmit={(event) => {
                        event.preventDefault();
                        void send();
                    }}
                >
                    <AttachmentList attachments={attachments} disabled={sending} onRemove={remove} />
                    <Text as="label" htmlFor={fieldId} visuallyHidden>
                        Feedback
                    </Text>
                    <ComposerField
                        ref={textareaRef}
                        disabled={sending}
                        id={fieldId}
                        onChange={(event) => {
                            setMessage(event.target.value);
                            setError(null);
                        }}
                        onKeyDown={(event) => {
                            if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
                                event.preventDefault();
                                void send();
                            }
                        }}
                        placeholder="What’s working and what isn’t…"
                        value={message}
                    />
                    <ComposerActions canSend={canSend} onFiles={add} sending={sending} />
                    {dragging ? <ComposerDropOverlay>Drop images to attach</ComposerDropOverlay> : null}
                </Composer>
                <ComposerStatus
                    fallback="Your feedback includes your handle, this page and details about your browser."
                    status={error ?? notice}
                />
            </ComposerDialog>
        </Dialog.Root>
    );
}

function acceptImages(current: Attachment[], incoming: File[]) {
    const images = incoming.filter((file) => FEEDBACK_IMAGE_TYPES.includes(file.type));
    const accepted: File[] = [];
    let bytes = current.reduce((sum, a) => sum + a.file.size, 0);
    for (const file of images) {
        if (current.length + accepted.length === MAX_FEEDBACK_IMAGES) {
            return { accepted, refused: `Attach up to ${MAX_FEEDBACK_IMAGES} images.` };
        }
        if (bytes + file.size > MAX_FEEDBACK_IMAGE_BYTES) {
            return { accepted, refused: `Images can add up to ${MAX_FEEDBACK_IMAGE_BYTES / 1024 / 1024} MB.` };
        }
        bytes += file.size;
        accepted.push(file);
    }
    return {
        accepted,
        refused: images.length < incoming.length ? 'Only PNG, JPEG, GIF and WebP images can be attached.' : null,
    };
}

function AttachmentList({
    attachments,
    disabled,
    onRemove,
}: {
    attachments: Attachment[];
    disabled: boolean;
    onRemove: (gone: Attachment) => void;
}) {
    const reduceMotion = useReducedMotion();
    return (
        <AnimatePresence initial={false}>
            {attachments.length > 0 ? (
                <MotionFrame
                    key="thumbs"
                    aria-label="Attached images"
                    as="ul"
                    direction="row"
                    gap="s"
                    paddingTop="m"
                    paddingX="m"
                    wrap
                    initial={reduceMotion ? false : { opacity: 0 }}
                    animate={{ opacity: 1 }}
                    exit={reduceMotion ? undefined : { opacity: 0 }}
                    transition={THUMB}
                >
                    <AnimatePresence initial={false} mode="popLayout">
                        {attachments.map((a) => (
                            <MotionFrame
                                key={a.id}
                                as="li"
                                layout={!reduceMotion}
                                initial={reduceMotion ? false : { opacity: 0, scale: 0.96 }}
                                animate={{ opacity: 1, scale: 1 }}
                                exit={reduceMotion ? undefined : { opacity: 0, scale: 0.96 }}
                                transition={THUMB}
                            >
                                <AttachmentThumb
                                    disabled={disabled}
                                    name={a.file.name}
                                    onRemove={() => onRemove(a)}
                                    src={a.url}
                                />
                            </MotionFrame>
                        ))}
                    </AnimatePresence>
                </MotionFrame>
            ) : null}
        </AnimatePresence>
    );
}

function ComposerActions({
    canSend,
    onFiles,
    sending,
}: {
    canSend: boolean;
    onFiles: (files: Iterable<File>) => void;
    sending: boolean;
}) {
    const fileRef = useRef<HTMLInputElement>(null);
    return (
        <Frame align="center" direction="row" gap="s" justify="between" paddingBottom="s" paddingX="s">
            <Frame align="center" direction="row" gap="xs">
                <Button
                    aria-label="Attach images"
                    disabled={sending}
                    onClick={() => fileRef.current?.click()}
                    size="icon-sm"
                    type="button"
                    variant="subtle"
                >
                    <ImageIcon size="xl" />
                </Button>
                <ComposerHint>Paste or drop images</ComposerHint>
                <input
                    ref={fileRef}
                    accept={FEEDBACK_IMAGE_TYPES.join(',')}
                    hidden
                    multiple
                    onChange={(event) => {
                        onFiles(event.target.files ?? []);
                        event.target.value = '';
                    }}
                    type="file"
                />
            </Frame>
            <Frame align="center" direction="row" gap="m">
                <Kbd hideOnTouch>{SEND_SHORTCUT}</Kbd>
                <Button aria-label="Send feedback" disabled={!canSend} size="icon-sm-round" type="submit">
                    {sending ? <LoadingIcon size="md" spin /> : <ArrowUpIcon size="md" />}
                </Button>
            </Frame>
        </Frame>
    );
}

function useImageIntake(add: (files: Iterable<File>) => void) {
    const [depth, setDepth] = useState(0);
    const carriesFiles = (event: DragEvent) => event.dataTransfer.types.includes('Files');
    return {
        dragging: depth > 0,
        intakeHandlers: {
            onPaste: (event: ClipboardEvent) => {
                const files = [...event.clipboardData.files];
                if (files.length === 0) return;
                event.preventDefault();
                add(files);
            },
            onDragEnter: (event: DragEvent) => {
                if (!carriesFiles(event)) return;
                event.preventDefault();
                setDepth((d) => d + 1);
            },
            onDragOver: (event: DragEvent) => {
                if (carriesFiles(event)) event.preventDefault();
            },
            onDragLeave: (event: DragEvent) => {
                if (carriesFiles(event)) setDepth((d) => Math.max(0, d - 1));
            },
            onDrop: (event: DragEvent) => {
                if (!carriesFiles(event)) return;
                event.preventDefault();
                setDepth(0);
                add(event.dataTransfer.files);
            },
        },
    };
}
