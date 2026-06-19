import type { KeyboardEvent } from 'react';
import { useRef, useState } from 'react';

import {
    Combobox,
    ComboboxChip,
    ComboboxChips,
    ComboboxChipsInput,
    ComboboxContent,
    ComboboxEmpty,
    ComboboxItem,
    ComboboxList,
    ComboboxValue,
} from '@/components/ui/combobox';

type Props = {
    value: string[];
    onValueChange: (next: string[]) => void;
    suggestions: string[];
    placeholder?: string;
    max?: number;
    id?: string;
    'aria-labelledby'?: string;
};

const DEFAULT_MAX = 10;

function normalizeTags(tags: string[], max: number): string[] {
    const out: string[] = [];
    const seen = new Set<string>();
    for (const raw of tags) {
        const tag = raw.trim();
        if (!tag) continue;
        const key = tag.toLowerCase();
        if (seen.has(key)) continue;
        seen.add(key);
        out.push(tag);
        if (out.length === max) break;
    }
    return out;
}

// CreatableCombobox is a multi-select tag input: pick from `suggestions`, or type
// a new tag and press Enter (or click "Create …") to add it. Enter never submits
// the surrounding form — the tag editor owns that key.
export function CreatableCombobox({
    value,
    onValueChange,
    suggestions,
    placeholder,
    max = DEFAULT_MAX,
    id,
    'aria-labelledby': ariaLabelledBy,
}: Props) {
    const [query, setQuery] = useState('');
    const anchorRef = useRef<HTMLDivElement | null>(null);
    const highlightedRef = useRef<string | null>(null);

    const trimmed = query.trim();
    const lowered = trimmed.toLowerCase();
    const selectedLower = new Set(value.map((tag) => tag.toLowerCase()));
    const unselected = suggestions.filter(
        (tag) => !selectedLower.has(tag.toLowerCase()),
    );
    const hasExact =
        selectedLower.has(lowered) ||
        suggestions.some((tag) => tag.toLowerCase() === lowered);
    const atMax = value.length >= max;
    const showCreate = trimmed !== '' && !hasExact && !atMax;
    const items = showCreate ? [...unselected, trimmed] : unselected;

    const commit = (next: string[]) => {
        onValueChange(normalizeTags(next, max));
        setQuery('');
    };

    const addTag = (tag: string) => {
        const next = tag.trim();
        if (!next || atMax || selectedLower.has(next.toLowerCase())) {
            return;
        }
        commit([...value, next]);
    };

    const onInputKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
        if (event.key !== 'Enter' || event.nativeEvent.isComposing) {
            return;
        }
        // An item is highlighted — let the combobox select it.
        if (highlightedRef.current) {
            return;
        }
        // Nothing highlighted: claim Enter so it never submits the parent form.
        event.preventDefault();
        event.stopPropagation();
        if (trimmed) {
            addTag(trimmed);
        }
    };

    return (
        <Combobox
            items={items}
            multiple
            value={value}
            onValueChange={commit}
            inputValue={query}
            onInputValueChange={setQuery}
            onItemHighlighted={(item) => {
                highlightedRef.current = (item as string | null) ?? null;
            }}
        >
            <ComboboxChips ref={anchorRef} className="rounded-xl">
                <ComboboxValue>
                    {(selected: string[]) => (
                        <>
                            {selected.map((tag) => (
                                <ComboboxChip key={tag} aria-label={tag}>
                                    {tag}
                                </ComboboxChip>
                            ))}
                            <ComboboxChipsInput
                                id={id}
                                aria-labelledby={ariaLabelledBy}
                                placeholder={
                                    selected.length === 0
                                        ? placeholder
                                        : undefined
                                }
                                onKeyDown={onInputKeyDown}
                            />
                        </>
                    )}
                </ComboboxValue>
            </ComboboxChips>
            <ComboboxContent anchor={anchorRef}>
                <ComboboxEmpty>No matches.</ComboboxEmpty>
                <ComboboxList>
                    {(item: string) => (
                        <ComboboxItem key={item} value={item}>
                            {showCreate && item === trimmed
                                ? `Create "${trimmed}"`
                                : item}
                        </ComboboxItem>
                    )}
                </ComboboxList>
            </ComboboxContent>
        </Combobox>
    );
}
