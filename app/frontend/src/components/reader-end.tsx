import { useEffect, useRef, useState, type ReactNode, type RefObject } from 'react';

import { TagIcon, TrashIcon } from '@/components/icons';
import { SaveTagEditor } from '@/components/save-tags';
import { TagChips } from '@/components/tag-input';
import { Button } from '@/components/ui/button';
import { MarkLine } from '@/components/ui/mark-line';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { Frame } from '@/design/frame';
import { useSaveState } from '@/hooks/use-entry-save';
import type { EntrySave } from '@/lib/entry-save';
import { savedPhrase } from '@/lib/library';
import { announceRemove } from '@/lib/save-announce';

const END = 'End of article';
export function ReaderEnd({ save }: { save: EntrySave | null }) {
    return (
        <Frame gap="l" paddingTop="6xl">
            {save ? <SaveEnd save={save} /> : <MarkLine label={END} />}
        </Frame>
    );
}

function SaveEnd({ save }: { save: EntrySave }) {
    const { saved, tags, savedAt } = useSaveState(save);
    return saved ? <SavedEnd save={save} savedAt={savedAt} tags={tags} /> : <MarkLine label={END} />;
}
function SavedEnd({ save, tags, savedAt }: { save: EntrySave; tags: string[]; savedAt: string | null }) {
    const [editing, setEditing] = useState(false);
    const input = useRef<HTMLInputElement>(null);
    useEffect(() => {
        if (editing) input.current?.focus();
    }, [editing]);
    const tagLabel = tags.length ? 'Edit tags' : 'Add tags';

    return (
        <>
            <MarkLine
                actions={
                    <>
                        <EndAction label={tagLabel} onClick={() => setEditing(!editing)} pressed={editing}>
                            <TagIcon />
                        </EndAction>
                        <EndAction label="Remove from Library" onClick={() => announceRemove(save, tags)}>
                            <TrashIcon />
                        </EndAction>
                    </>
                }
                label={END}
            >
                {savedPhrase(savedAt ?? new Date().toISOString())}
            </MarkLine>
            <SavedTags editing={editing} inputRef={input} save={save} tags={tags} />
        </>
    );
}

function SavedTags({
    save,
    tags,
    editing,
    inputRef,
}: {
    save: EntrySave;
    tags: string[];
    editing: boolean;
    inputRef: RefObject<HTMLInputElement | null>;
}) {
    if (editing)
        return (
            <Frame maxWidth="sm">
                <SaveTagEditor inputRef={inputRef} save={save} />
            </Frame>
        );
    if (!tags.length) return null;
    return (
        <TagChips
            inputRef={inputRef}
            onRemove={(tag) => save.setTags(tags.filter((other) => other !== tag))}
            tags={tags}
        />
    );
}

function EndAction({
    label,
    onClick,
    pressed,
    children,
}: {
    label: string;
    onClick: () => void;
    pressed?: boolean;
    children: ReactNode;
}) {
    return (
        <Tooltip>
            <TooltipTrigger
                render={
                    <Button aria-label={label} aria-pressed={pressed} onClick={onClick} size="icon-sm" variant="subtle">
                        {children}
                    </Button>
                }
            />
            <TooltipContent>{label}</TooltipContent>
        </Tooltip>
    );
}
