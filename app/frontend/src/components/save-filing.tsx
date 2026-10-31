import { useEffect, useRef, useState } from 'react';

import { CheckIcon } from '@/components/icons';
import { SaveTagEditor } from '@/components/save-tags';
import { announceRemove } from '@/lib/save-announce';
import { TagChips } from '@/components/tag-input';
import { useSaveState } from '@/hooks/use-entry-save';
import type { EntrySave } from '@/lib/entry-save';

const ACTION = '-my-3 cursor-pointer rounded-sm py-3 text-sm text-muted-foreground hover:text-foreground';

// The end of a saved article is where it gets filed; an unsaved one ends without asking.
export function SaveFiling({ save }: { save: EntrySave }) {
    const { saved, tags } = useSaveState(save);
    return saved ? <SavedFiling save={save} tags={tags} /> : null;
}

// Unmounts with the save, so a re-save starts closed however it was removed.
function SavedFiling({ save, tags }: { save: EntrySave; tags: string[] }) {
    const [editing, setEditing] = useState(false);
    const input = useRef<HTMLInputElement>(null);
    useEffect(() => {
        if (editing) input.current?.focus();
    }, [editing]);

    return (
        <section aria-labelledby="save-filing" className="mt-16 border-t border-border pt-8">
            <div className="flex flex-wrap items-center justify-between gap-3">
                <h2 className="flex items-center gap-2 text-sm font-medium" id="save-filing">
                    <CheckIcon className="size-4 text-success-icon" />
                    Saved to Library
                </h2>
                <div className="flex items-center gap-3">
                    {!editing ? (
                        <>
                            <button className={ACTION} onClick={() => setEditing(true)} type="button">Add tags</button>
                            <span aria-hidden="true" className="h-4 w-px bg-border" />
                        </>
                    ) : null}
                    <button className={ACTION} onClick={() => announceRemove(save, tags)} type="button">Remove</button>
                </div>
            </div>
            {editing ? (
                <div className="mt-6 max-w-sm"><SaveTagEditor inputRef={input} save={save} /></div>
            ) : tags.length ? (
                <TagChips inputRef={input} onRemove={(tag) => save.setTags(tags.filter((other) => other !== tag))} tags={tags} />
            ) : null}
        </section>
    );
}
