import { useEffect, useRef, useState } from 'react';

import { CheckIcon } from '@/components/icons';
import { SaveTagEditor } from '@/components/save-tags';
import { announceRemove } from '@/lib/save-announce';
import { TagChips } from '@/components/tag-input';
import { Button } from '@/components/ui/button';
import { Separator } from '@/components/ui/separator';
import { Frame } from '@/design/frame';
import { Text } from '@/design/text';
import { useSaveState } from '@/hooks/use-entry-save';
import type { EntrySave } from '@/lib/entry-save';

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
        <Frame paddingTop="6xl">
            <Frame aria-labelledby="save-filing" as="section" border="top" paddingTop="3xl">
                <Frame align="center" direction="row" gap="m" justify="between" wrap>
                    <Frame align="center" direction="row" gap="s">
                        <CheckIcon size="md" tone="success-icon" />
                        <Text as="h2" id="save-filing" size="sm" weight="medium">Saved to Library</Text>
                    </Frame>
                    <Frame align="center" direction="row" gap="m">
                        {!editing ? (
                            <>
                                <Button onClick={() => setEditing(true)} size="bare" variant="nav">Add tags</Button>
                                <Separator />
                            </>
                        ) : null}
                        <Button onClick={() => announceRemove(save, tags)} size="bare" variant="nav">Remove</Button>
                    </Frame>
                </Frame>
                {editing ? (
                    <Frame maxWidth="sm" paddingTop="2xl"><SaveTagEditor inputRef={input} save={save} /></Frame>
                ) : tags.length ? (
                    <Frame paddingTop="m"><TagChips inputRef={input} onRemove={(tag) => save.setTags(tags.filter((other) => other !== tag))} tags={tags} /></Frame>
                ) : null}
            </Frame>
        </Frame>
    );
}
