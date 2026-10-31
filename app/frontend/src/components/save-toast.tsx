import { useEffect, useRef, useState } from 'react';

import { CheckIcon } from '@/components/icons';
import { SaveTagEditor } from '@/components/save-tags';
import { useShortcuts } from '@/hooks/use-shortcuts';
import type { EntrySave } from '@/lib/entry-save';

// Tagging is offered, never asked for: the field opens on "Add tags" or T, so saving never takes the reader's keys.
export function SaveToast({ save, onHold }: { save: EntrySave; onHold: (held: boolean) => void }) {
    const [open, setOpen] = useState(false);
    const input = useRef<HTMLInputElement>(null);
    useShortcuts({ t: () => setOpen(true) });
    useEffect(() => {
        if (open) input.current?.focus();
    }, [open]);

    return (
        <div
            className="flex w-full min-w-0 flex-col gap-3"
            onBlur={(event) => { if (!event.currentTarget.contains(event.relatedTarget)) onHold(false); }}
            onFocus={() => onHold(true)}
        >
            <div className="flex items-center gap-3">
                <CheckIcon className="size-4 shrink-0 text-success-icon" />
                <span className="flex-1 font-medium">Saved to Library</span>
                {!open ? (
                    <button
                        aria-keyshortcuts="t"
                        className="-my-1 inline-flex cursor-pointer items-center gap-2 rounded-sm px-2 py-1 text-muted-foreground hover:bg-secondary hover:text-foreground"
                        onClick={() => setOpen(true)}
                        type="button"
                    >
                        Add tags
                        <kbd className="hidden rounded-sm px-1 font-sans text-xs text-subtle-foreground shadow-[inset_0_0_0_1px_var(--border)] [@media(pointer:fine)]:inline">T</kbd>
                    </button>
                ) : null}
            </div>
            {open ? <div><SaveTagEditor inputRef={input} save={save} /></div> : null}
        </div>
    );
}
