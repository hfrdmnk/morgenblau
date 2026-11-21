import { useId, useRef, type ReactNode, type RefObject } from 'react';

import { CloseIcon, EnterIcon } from '@/components/icons';
import { Combobox, ComboboxContent, ComboboxInput, ComboboxItem, ComboboxList } from '@/components/ui/combobox';
import { Button } from '@/components/ui/button';
import { InputGroup, InputGroupButton, InputGroupInput } from '@/components/ui/input-group';
import { Frame } from '@/design/frame';
import { Text } from '@/design/text';
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
    if (!tags.length) return <Text size="sm" tone="subtle">No tags yet.</Text>;
    return (
        <Frame aria-label="Tags" as="ul" direction="row" gap="s" wrap>
            {tags.map((tag) => (
                <Frame align="center" as="li" background="muted" direction="row" gap="2xs" key={tag} paddingEnd="2xs" paddingStart="m" paddingY="3xs" radius="full">
                    <Text as="span" size="sm" tone="muted">{tag}</Text>
                    <Button
                        aria-label={`Remove tag ${tag}`}
                        onClick={(event) => remove(tag, event.currentTarget)}
                        size="icon-xs"
                        type="button"
                        variant="quiet"
                    >
                        <CloseIcon size="sm" />
                    </Button>
                </Frame>
            ))}
        </Frame>
    );
}

export function TagInput({ tags, draft, canAdd, overlong, inputRef, onDraft, onAdd, suggestionsPath = '/api/subscriptions/tags', hiddenLabel = false }: {
    tags: string[];
    draft: string;
    canAdd: boolean;
    overlong: boolean;
    inputRef: RefObject<HTMLInputElement | null>;
    onDraft: (draft: string) => void;
    onAdd: (input: string) => void;
    suggestionsPath?: string;
    hiddenLabel?: boolean;
}) {
    const inputId = useId();
    const hintId = useId();
    const anchor = useRef<HTMLDivElement>(null);
    // Enter on a highlighted suggestion belongs to the combobox, which runs this input's handler first.
    const highlighted = useRef(false);
    const suggestions = useTagSuggestions(suggestionsPath, tags);
    const full = tags.length >= MAX_TAGS;
    return (
        <Frame gap="s">
            {hiddenLabel ? (
                <Text as="label" htmlFor={inputId} visuallyHidden>Add a tag</Text>
            ) : (
                <Text as="label" htmlFor={inputId} size="sm" weight="medium">Add a tag</Text>
            )}
            <Combobox
                inputValue={draft}
                items={suggestions}
                onInputValueChange={onDraft}
                onItemHighlighted={(item) => { highlighted.current = item !== undefined; }}
                onValueChange={(tag: string | null) => { if (tag) onAdd(tag); }}
                value={null}
            >
                <InputGroup ref={anchor}>
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
                        placeholder={full ? `Up to ${MAX_TAGS} tags` : hiddenLabel ? 'Add a tag' : 'e.g. design'}
                        ref={inputRef}
                        render={<InputGroupInput />}
                    />
                    <InputGroupButton aria-label="Add tag" disabled={!canAdd} onClick={() => onAdd(draft)}>
                        <EnterIcon />
                    </InputGroupButton>
                </InputGroup>
                <ComboboxContent anchor={anchor} hideEmpty>
                    <ComboboxList>
                        {(tag: string) => <ComboboxItem key={tag} value={tag}>{tag}</ComboboxItem>}
                    </ComboboxList>
                </ComboboxContent>
            </Combobox>
            <LimitHint id={hintId} shown={overlong}>Tags can be up to {MAX_TAG_LENGTH} characters.</LimitHint>
        </Frame>
    );
}

function useTagSuggestions(path: string, exclude: string[]) {
    const state = useSources<{ tags: string[] }>(path);
    const taken = new Set(exclude.map((tag) => tag.toLowerCase()));
    return state.status === 'loaded' ? state.data.tags.filter((tag) => !taken.has(tag.toLowerCase())) : [];
}

function LimitHint({ id, shown, children }: { id: string; shown: boolean; children: ReactNode }) {
    if (!shown) return null;
    return <Text id={id} size="sm" tone="destructive">{children}</Text>;
}
