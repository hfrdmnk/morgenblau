import { Dialog } from '@base-ui/react/dialog';
import { AnimatePresence, motion, useReducedMotion } from 'motion/react';
import { useEffect, useId, useRef, useState, type ClipboardEvent, type DragEvent } from 'react';
import { toast } from 'sonner';

import { ArrowUpIcon, CloseIcon, FeedbackIcon, ImageIcon, LoadingIcon } from '@/components/icons';
import { Button } from '@/components/ui/button';
import { DialogBackdrop, DialogPopup } from '@/components/ui/dialog';
import { describeMutationError } from '@/lib/api';
import { FEEDBACK_IMAGE_TYPES, MAX_FEEDBACK_IMAGE_BYTES, MAX_FEEDBACK_IMAGES, sendFeedback } from '@/lib/feedback';
import { cn } from '@/lib/utils';

const THUMB = { duration: 0.16, ease: [0.16, 1, 0.3, 1] as const };
const SEND_SHORTCUT = /Mac|iPhone|iPad/.test(navigator.platform) ? '⌘ Enter' : 'Ctrl Enter';

type Attachment = { id: string; file: File; url: string };

// One composer surface, like a chat input: thumbnails above the text, attach and send below it. The draft outlives a close.
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
            await sendFeedback(message.trim(), attachments.map((a) => a.file));
            urls.current.forEach((url) => URL.revokeObjectURL(url));
            urls.current.clear();
            setAttachments([]);
            setMessage('');
            setNotice(null);
            setOpen(false);
            toast.success('Feedback sent. Thank you.');
        } catch (failure) {
            setError(describeMutationError(failure, 'Couldn’t send your feedback. Check your connection and try again.'));
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
                    <Button aria-label="Send feedback" className="text-subtle-foreground" size="icon-sm" variant="ghost">
                        <FeedbackIcon className="size-5" />
                    </Button>
                }
            />
            <Dialog.Portal>
                <DialogBackdrop />
                <DialogPopup className="max-w-xl p-2" initialFocus={textareaRef} {...intakeHandlers}>
                    <Dialog.Title className="sr-only">Send feedback</Dialog.Title>
                    <form
                        className={cn(
                            'relative rounded-lg bg-input transition-[box-shadow] duration-(--motion-duration-fast) has-[textarea:focus-visible]:shadow-[inset_0_0_0_1px_var(--ring)]',
                            dragging && 'shadow-[inset_0_0_0_1px_var(--ring)]',
                        )}
                        onSubmit={(event) => {
                            event.preventDefault();
                            void send();
                        }}
                    >
                        <AttachmentList attachments={attachments} disabled={sending} onRemove={remove} />
                        <label className="sr-only" htmlFor={fieldId}>
                            Feedback
                        </label>
                        <textarea
                            ref={textareaRef}
                            className="field-sizing-content block max-h-[50dvh] min-h-32 w-full resize-none bg-transparent px-4 pt-3.5 pb-2 text-base outline-none placeholder:text-subtle-foreground md:text-sm"
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
                            placeholder="What’s working, what isn’t, what you’d like to see…"
                            value={message}
                        />
                        <ComposerActions canSend={canSend} onFiles={add} sending={sending} />
                        {dragging ? (
                            <div className="pointer-events-none absolute inset-0 flex items-center justify-center gap-2 rounded-lg bg-input/90 text-sm text-muted-foreground outline-1 outline-ring outline-dashed -outline-offset-4">
                                <ImageIcon className="size-5" />
                                Drop images to attach
                            </div>
                        ) : null}
                    </form>
                    <StatusLine status={error ?? notice} />
                </DialogPopup>
            </Dialog.Portal>
        </Dialog.Root>
    );
}

// Takes images in order until the count or size limit; the first refusal explains why the rest stayed out.
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
    return { accepted, refused: images.length < incoming.length ? 'Only PNG, JPEG, GIF and WebP images can be attached.' : null };
}

// Thumbnails above the text, the way a chat composer shows what it will send.
function AttachmentList({ attachments, disabled, onRemove }: { attachments: Attachment[]; disabled: boolean; onRemove: (gone: Attachment) => void }) {
    const reduceMotion = useReducedMotion();
    return (
        <AnimatePresence initial={false}>
            {attachments.length > 0 ? (
                <motion.ul
                    key="thumbs"
                    aria-label="Attached images"
                    className="flex flex-wrap gap-2 px-3 pt-3"
                    initial={reduceMotion ? false : { opacity: 0 }}
                    animate={{ opacity: 1 }}
                    exit={reduceMotion ? undefined : { opacity: 0 }}
                    transition={THUMB}
                >
                    <AnimatePresence initial={false} mode="popLayout">
                        {attachments.map((a) => (
                            <motion.li
                                key={a.id}
                                layout={!reduceMotion}
                                className="group relative"
                                initial={reduceMotion ? false : { opacity: 0, scale: 0.96 }}
                                animate={{ opacity: 1, scale: 1 }}
                                exit={reduceMotion ? undefined : { opacity: 0, scale: 0.96 }}
                                transition={THUMB}
                            >
                                <img
                                    alt={a.file.name}
                                    className="size-16 rounded-md object-cover shadow-[0_0_0_1px_oklch(from_var(--foreground)_l_c_h/0.08)]"
                                    height={64}
                                    src={a.url}
                                    width={64}
                                />
                                <button
                                    aria-label={`Remove ${a.file.name}`}
                                    className="absolute -top-1.5 -right-1.5 flex size-5 cursor-pointer items-center justify-center rounded-full bg-primary text-primary-foreground opacity-0 shadow-popover transition-opacity duration-(--motion-duration-fast) group-hover:opacity-100 group-focus-within:opacity-100 pointer-coarse:opacity-100 before:absolute before:-inset-2.5 focus-visible:opacity-100"
                                    disabled={disabled}
                                    onClick={() => onRemove(a)}
                                    type="button"
                                >
                                    <CloseIcon className="size-3" />
                                </button>
                            </motion.li>
                        ))}
                    </AnimatePresence>
                </motion.ul>
            ) : null}
        </AnimatePresence>
    );
}

function ComposerActions({ canSend, onFiles, sending }: { canSend: boolean; onFiles: (files: Iterable<File>) => void; sending: boolean }) {
    const fileRef = useRef<HTMLInputElement>(null);
    return (
        <div className="flex items-center justify-between gap-2 px-2 pb-2">
            <div className="flex items-center gap-1.5">
                <Button
                    aria-label="Attach images"
                    className="text-subtle-foreground hover:text-foreground"
                    disabled={sending}
                    onClick={() => fileRef.current?.click()}
                    size="icon-sm"
                    type="button"
                    variant="ghost"
                >
                    <ImageIcon className="size-5" />
                </Button>
                <span className="hidden text-xs text-subtle-foreground pointer-fine:inline">Paste or drop images</span>
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
            </div>
            <div className="flex items-center gap-3">
                <kbd className="hidden font-sans text-xs text-subtle-foreground pointer-fine:inline">{SEND_SHORTCUT}</kbd>
                <Button aria-label="Send feedback" className="rounded-full" disabled={!canSend} size="icon-sm" type="submit">
                    {sending ? <LoadingIcon className="size-4 motion-safe:animate-spin" /> : <ArrowUpIcon className="size-4" />}
                </Button>
            </div>
        </div>
    );
}

// Says what gets sent along until there is a refusal or a failure to report.
function StatusLine({ status }: { status: string | null }) {
    return (
        <p className={cn('min-h-lh px-2 pt-2 pb-1 text-xs', status ? 'text-destructive' : 'text-subtle-foreground')} role={status ? 'alert' : undefined}>
            {status ?? 'Your handle, this page and your browser are automatically included to help track down problems.'}
        </p>
    );
}

// Images arrive by paste or drop anywhere in the dialog. Drags count nested enter/leave pairs, since dragging over a child fires leave on its parent.
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
