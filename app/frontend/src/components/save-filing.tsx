import { useEffect, useRef, useState, type RefObject } from 'react';

import { SaveTagEditor } from '@/components/save-tags';
import { announceRemove } from '@/lib/save-announce';
import { TagChips } from '@/components/tag-input';
import { Button } from '@/components/ui/button';
import { Separator } from '@/components/ui/separator';
import { Frame } from '@/design/frame';
import { Text } from '@/design/text';
import { useSaveState } from '@/hooks/use-entry-save';
import type { EntrySave } from '@/lib/entry-save';
import { saveState, visibleTags, withState, type SaveState } from '@/lib/library';

export function SaveFiling({ save }: { save: EntrySave }) {
    const { saved, tags } = useSaveState(save);
    return saved ? <SavedFiling save={save} tags={tags} /> : null;
}

const FILED: Record<SaveState, string> = { queue: 'Saved to Library', archived: 'Archived' };
const NEXT: Record<SaveState, { label: string; state: SaveState }> = {
    queue: { label: 'Archive', state: 'archived' },
    archived: { label: 'Read again later', state: 'queue' },
};

function SavedFiling({ save, tags }: { save: EntrySave; tags: string[] }) {
    const state = saveState(tags);
    const next = NEXT[state];
    const [editing, setEditing] = useState(false);
    const input = useRef<HTMLInputElement>(null);
    useEffect(() => {
        if (editing) input.current?.focus();
    }, [editing]);

    return (
        <Frame paddingTop="5xl">
            <Frame aria-label={FILED[state]} as="section" border="top" gap="l" paddingTop="3xl">
                <Frame align="center" direction="row" gapX="m" gapY="xs" wrap>
                    <Text as="span" size="sm" tone="subtle">
                        {FILED[state]}
                    </Text>
                    <Frame align="center" direction="row" gap="m">
                        <Button onClick={() => save.setTags(withState(tags, next.state))} size="bare" variant="nav">
                            {next.label}
                        </Button>
                        <Separator />
                        <Button disabled={editing} onClick={() => setEditing(true)} size="bare" variant="nav">
                            {visibleTags(tags).length ? 'Edit tags' : 'Add tags'}
                        </Button>
                        <Separator />
                        <Button onClick={() => announceRemove(save, tags)} size="bare" variant="nav">
                            Remove from Library
                        </Button>
                    </Frame>
                </Frame>
                <SavedTags editing={editing} input={input} save={save} tags={tags} />
            </Frame>
        </Frame>
    );
}

function SavedTags({
    save,
    tags,
    editing,
    input,
}: {
    save: EntrySave;
    tags: string[];
    editing: boolean;
    input: RefObject<HTMLInputElement | null>;
}) {
    const shown = visibleTags(tags);
    if (editing)
        return (
            <Frame maxWidth="sm">
                <SaveTagEditor inputRef={input} save={save} />
            </Frame>
        );
    if (!shown.length) return null;
    return (
        <TagChips
            inputRef={input}
            onRemove={(tag) => save.setTags(tags.filter((other) => other !== tag))}
            tags={shown}
        />
    );
}
