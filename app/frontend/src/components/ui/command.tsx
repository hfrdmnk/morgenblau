import { Dialog } from '@base-ui/react/dialog';
import { Command as Cmdk } from 'cmdk';
import type { ComponentProps } from 'react';

import { CheckIcon, ChevronDownIcon } from '@/components/icons';
import { DialogBackdrop, DialogPopup } from '@/components/ui/dialog';
import { cn } from 'cn';

function CommandDialog({
    title,
    children,
    ...props
}: Omit<Dialog.Popup.Props, 'className' | 'style'> & { title: string }) {
    return (
        <Dialog.Portal>
            <DialogBackdrop className="transition-none" />
            <DialogPopup
                className="top-(--palette-top) max-w-xl translate-y-0 overflow-hidden p-0 transition-none"
                {...props}
            >
                <Dialog.Title className="sr-only">{title}</Dialog.Title>
                {children}
            </DialogPopup>
        </Dialog.Portal>
    );
}

function CommandSearch({
    chip,
    ...props
}: Omit<ComponentProps<typeof Cmdk.Input>, 'className' | 'style'> & { chip?: string }) {
    return (
        <div className="flex h-14 items-center gap-2 border-b border-border px-5">
            {chip ? (
                <span className="flex shrink-0 items-center gap-1.5 text-muted-foreground motion-safe:animate-in motion-safe:fade-in-0 motion-safe:slide-in-from-left-1 motion-safe:duration-(--motion-duration-ui)">
                    {chip}
                    <ChevronDownIcon aria-hidden="true" rotate={270} size="sm" tone="subtle" />
                </span>
            ) : null}
            <Cmdk.Input
                className="h-full min-w-0 flex-1 bg-transparent text-base outline-none placeholder:text-subtle-foreground"
                {...props}
            />
        </div>
    );
}

function CommandList(props: Omit<ComponentProps<typeof Cmdk.List>, 'className' | 'style'>) {
    return (
        <Cmdk.List
            className="max-h-(--palette-max-height) scroll-py-2 overflow-y-auto overscroll-contain p-2"
            {...props}
        />
    );
}

function CommandGroup(props: Omit<ComponentProps<typeof Cmdk.Group>, 'className' | 'style'>) {
    return (
        <Cmdk.Group
            className="px-0 [&_[cmdk-group-heading]]:px-3 [&_[cmdk-group-heading]]:pt-3 [&_[cmdk-group-heading]]:pb-1.5 [&_[cmdk-group-heading]]:text-xs [&_[cmdk-group-heading]]:font-medium [&_[cmdk-group-heading]]:text-subtle-foreground"
            {...props}
        />
    );
}

function CommandItem({
    destructive = false,
    ...props
}: Omit<ComponentProps<typeof Cmdk.Item>, 'className' | 'style'> & { destructive?: boolean }) {
    return (
        <Cmdk.Item
            className={cn(
                'flex min-h-10 cursor-default items-center gap-3 rounded-md px-3 text-sm select-none data-[disabled=true]:opacity-50 data-[selected=true]:bg-secondary',
                destructive && 'text-destructive',
            )}
            {...props}
        />
    );
}

function CommandToggle({ on }: { on: boolean }) {
    return (
        <span
            aria-hidden="true"
            className={cn(
                'flex h-4 w-7 shrink-0 rounded-full p-0.5 transition-[background-color] duration-(--motion-duration-ui) ease-out',
                on ? 'bg-primary' : 'bg-accent',
            )}
        >
            <span
                className={cn(
                    'size-3 rounded-full bg-popover transition-transform duration-(--motion-duration-ui) ease-(--ease-out)',
                    on && 'translate-x-3',
                )}
            />
        </span>
    );
}

function CommandCheck({ checked }: { checked: boolean }) {
    return (
        <span className="flex size-4 shrink-0 items-center justify-center">
            {checked ? <CheckIcon aria-hidden="true" size="md" tone="primary" /> : null}
        </span>
    );
}

export { CommandCheck, CommandDialog, CommandGroup, CommandItem, CommandList, CommandSearch, CommandToggle };
