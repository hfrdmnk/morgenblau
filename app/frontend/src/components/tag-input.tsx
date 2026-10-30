import { useId, useRef, type ReactNode, type RefObject } from 'react';

import { CloseIcon, EnterIcon } from '@/components/icons';
import { Combobox, ComboboxContent, ComboboxInput, ComboboxItem, ComboboxList } from '@/components/ui/combobox';
import { InputGroup, InputGroupButton, InputGroupInput } from '@/components/ui/input-group';
import { useSources } from '@/hooks/use-sources';
import { MAX_TAG_LENGTH, MAX_TAGS } from '@/lib/source-edit';

export function TagChips({ tags, inputRef, onRemove }: {
    tags: string[];
    inputRef: RefObject<HTMLInputElement | null>;
    onRemove: (tag: string) => void;
}) {
    // The removed chip's button unmounts, so focus moves to a neighbour first instead of dropping to the page.
    function remove(tag: string, button: HTMLButtonElement) {
        const buttons = [...(button.closest('ul')?.querySelectorAll('button') ?? [])];
        const index = buttons.indexOf(button);
        (buttons[index + 1] ?? buttons[index - 1] ?? inputRef.current)?.focus();
        onRemove(tag);
    }
    if (!tags.length) return <p className="mt-3 text-sm text-subtle-foreground">No tags yet.</p>;
    return (
        <ul aria-label="Tags" className="mt-3 flex flex-wrap gap-2">
            {tags.map((tag) => (
                <li className="flex items-center gap-1 rounded-full bg-muted py-0.5 pr-1 pl-2.5 text-sm text-muted-foreground" key={tag}>
                    {tag}
                    <button
                        aria-label={`Remove tag ${tag}`}
                        className="relative flex size-5 cursor-pointer items-center justify-center rounded-full before:absolute before:-inset-1 hover:bg-secondary hover:text-foreground"
                        onClick={(event) => remove(tag, event.currentTarget)}
                        type="button"
                    >
                        <CloseIcon className="size-3.5" />
                    </button>
                </li>
            ))}
        </ul>
    );
}

export function TagInput({ tags, draft, canAdd, overlong, inputRef, onDraft, onAdd }: {
    tags: string[];
    draft: string;
    canAdd: boolean;
    overlong: boolean;
    inputRef: RefObject<HTMLInputElement | null>;
    onDraft: (draft: string) => void;
    onAdd: (input: string) => void;
}) {
    const inputId = useId();
    const hintId = useId();
    const anchor = useRef<HTMLDivElement>(null);
    // Enter on a highlighted suggestion belongs to the combobox, which runs this input's handler first.
    const highlighted = useRef(false);
    const suggestions = useTagSuggestions(tags);
    const full = tags.length >= MAX_TAGS;
    return (
        <>
            <label className="text-sm font-medium" htmlFor={inputId}>Add a tag</label>
            <Combobox
                inputValue={draft}
                items={suggestions}
                onInputValueChange={onDraft}
                onItemHighlighted={(item) => { highlighted.current = item !== undefined; }}
                onValueChange={(tag: string | null) => { if (tag) onAdd(tag); }}
                value={null}
            >
                <InputGroup className="mt-2" ref={anchor}>
                    <ComboboxInput
                        aria-describedby={hintId}
                        aria-invalid={overlong}
                        disabled={full}
                        id={inputId}
                        onKeyDown={(event) => {
                            if (event.key !== 'Enter' || highlighted.current) return;
                            event.preventDefault();
                            onAdd(draft);
                        }}
                        placeholder={full ? `Up to ${MAX_TAGS} tags` : 'e.g. design'}
                        ref={inputRef}
                        render={<InputGroupInput />}
                    />
                    <InputGroupButton aria-label="Add tag" disabled={!canAdd} onClick={() => onAdd(draft)}>
                        <EnterIcon />
                    </InputGroupButton>
                </InputGroup>
                <ComboboxContent anchor={anchor} className="data-empty:hidden">
                    <ComboboxList>
                        {(tag: string) => <ComboboxItem key={tag} value={tag}>{tag}</ComboboxItem>}
                    </ComboboxList>
                </ComboboxContent>
            </Combobox>
            <LimitHint id={hintId} shown={overlong}>Tags can be up to {MAX_TAG_LENGTH} characters.</LimitHint>
        </>
    );
}

function useTagSuggestions(exclude: string[]) {
    const state = useSources<{ tags: string[] }>('/api/subscriptions/tags');
    const taken = new Set(exclude.map((tag) => tag.toLowerCase()));
    return state.status === 'loaded' ? state.data.tags.filter((tag) => !taken.has(tag.toLowerCase())) : [];
}

export function LimitHint({ id, shown, children }: { id: string; shown: boolean; children: ReactNode }) {
    if (!shown) return null;
    return <p className="mt-2 text-sm text-destructive" id={id}>{children}</p>;
}
