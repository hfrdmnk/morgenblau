import { useState, type RefObject } from 'react';

import { TagChips, TagInput } from '@/components/tag-input';
import { useSaveState } from '@/hooks/use-entry-save';
import type { EntrySave } from '@/lib/entry-save';
import { addTags, canAddTags, hasOverlongTag } from '@/lib/source-edit';

export function SaveTagEditor({ save, inputRef }: { save: EntrySave; inputRef: RefObject<HTMLInputElement | null> }) {
    const { tags } = useSaveState(save);
    const [draft, setDraft] = useState('');
    return (
        <>
            <TagInput
                canAdd={canAddTags(tags, draft)}
                draft={draft}
                hiddenLabel
                inputRef={inputRef}
                onAdd={(input) => {
                    if (!canAddTags(tags, input)) return;
                    save.setTags(addTags(tags, input));
                    setDraft('');
                }}
                onDraft={setDraft}
                overlong={hasOverlongTag(draft)}
                suggestionsPath="/api/saves/tags"
                tags={tags}
            />
            {tags.length ? <TagChips inputRef={inputRef} onRemove={(tag) => save.setTags(tags.filter((other) => other !== tag))} tags={tags} /> : null}
        </>
    );
}
