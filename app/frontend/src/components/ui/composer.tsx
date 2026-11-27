import { Dialog } from '@base-ui/react/dialog';
import type { ComponentProps, ReactNode } from 'react';

import { CloseIcon, ImageIcon } from '@/components/icons';
import { DialogBackdrop, DialogPopup } from '@/components/ui/dialog';
import { cn } from 'cn';

function ComposerDialog({
    title,
    children,
    ...props
}: Omit<Dialog.Popup.Props, 'className' | 'style'> & { title: string }) {
    return (
        <Dialog.Portal>
            <DialogBackdrop />
            <DialogPopup className="max-w-xl p-2" {...props}>
                <Dialog.Title className="sr-only">{title}</Dialog.Title>
                {children}
            </DialogPopup>
        </Dialog.Portal>
    );
}

function Composer({ dragging, ...props }: Omit<ComponentProps<'form'>, 'className' | 'style'> & { dragging: boolean }) {
    return (
        <form
            className={cn(
                'relative flex flex-col rounded-lg bg-input transition-[box-shadow] duration-(--motion-duration-fast) has-[textarea:focus-visible]:shadow-hairline-ring',
                dragging && 'shadow-hairline-ring',
            )}
            {...props}
        />
    );
}

function ComposerField(props: Omit<ComponentProps<'textarea'>, 'className' | 'style'>) {
    return (
        <textarea
            className="field-sizing-content block max-h-(--composer-max-height) min-h-32 w-full resize-none bg-transparent px-4 pt-3.5 pb-2 text-base outline-none placeholder:text-subtle-foreground md:text-sm"
            {...props}
        />
    );
}

function ComposerDropOverlay({ children }: { children: ReactNode }) {
    return (
        <div className="pointer-events-none absolute inset-0 flex items-center justify-center gap-2 rounded-lg bg-input-veil text-sm text-muted-foreground outline-1 -outline-offset-4 outline-ring outline-dashed">
            <ImageIcon size="xl" />
            {children}
        </div>
    );
}

function ComposerHint({ children }: { children: ReactNode }) {
    return <span className="hidden text-xs text-subtle-foreground pointer-fine:inline">{children}</span>;
}

function AttachmentThumb({
    src,
    name,
    disabled,
    onRemove,
}: {
    src: string;
    name: string;
    disabled: boolean;
    onRemove: () => void;
}) {
    return (
        <span className="group relative block">
            <img
                alt={name}
                className="size-16 rounded-md object-cover shadow-image-edge"
                height={64}
                src={src}
                width={64}
            />
            <button
                aria-label={`Remove ${name}`}
                className="absolute -top-1.5 -right-1.5 flex size-5 cursor-pointer items-center justify-center rounded-full bg-primary text-primary-foreground opacity-0 shadow-popover transition-opacity duration-(--motion-duration-fast) group-focus-within:opacity-100 group-hover:opacity-100 before:absolute before:-inset-2.5 focus-visible:opacity-100 pointer-coarse:opacity-100"
                disabled={disabled}
                onClick={onRemove}
                type="button"
            >
                <CloseIcon size="xs" />
            </button>
        </span>
    );
}

function ComposerStatus({ status, fallback }: { status: string | null; fallback: string }) {
    return (
        <p
            className={cn('min-h-lh px-2 pt-2 pb-1 text-xs', status ? 'text-destructive' : 'text-subtle-foreground')}
            role={status ? 'alert' : undefined}
        >
            {status ?? fallback}
        </p>
    );
}

export { AttachmentThumb, Composer, ComposerDialog, ComposerDropOverlay, ComposerField, ComposerHint, ComposerStatus };
