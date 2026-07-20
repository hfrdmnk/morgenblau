import { SearchIcon } from '@proicons/react';
import { useEffect, useRef, useState } from 'react';

import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Badge } from '@/components/ui/badge';
import {
    Combobox,
    ComboboxContent,
    ComboboxEmpty,
    ComboboxInput,
    ComboboxItem,
    ComboboxList,
} from '@/components/ui/combobox';
import { InputGroupAddon } from '@/components/ui/input-group';
import { api } from '@/lib/api';
import { initialsFromHandle, personRowLines } from '@/lib/handle';
import {
    NO_SEARCH_RESULTS,
    personSearchView,
    searchResultHint,
    type PersonSearchResult,
    type PersonSearchResultsState,
    type PersonSearchView,
} from '@/lib/person-search';
import { safeHref } from '@/lib/utils';

const DEBOUNCE_MS = 250;

type Props = {
    onSelect: (result: PersonSearchResult) => void;
    onQueryChange: () => void;
};

// SPEC <discovery> line 558: whole-network search opens the People tab, replacing the bare
// handle-follow form. Search finds, it never follows — picking a result never sets it as the
// combobox's own value, since the panel materializes it as a standalone card instead; the field
// resets to empty so it's ready to search again.
export function PersonSearch({ onSelect, onQueryChange }: Props) {
    const [query, setQuery] = useState('');
    const [open, setOpen] = useState(false);
    const [resultsState, setResultsState] =
        useState<PersonSearchResultsState>(NO_SEARCH_RESULTS);
    const abortRef = useRef<AbortController | null>(null);
    const anchorRef = useRef<HTMLDivElement | null>(null);

    useEffect(() => {
        const trimmed = query.trim();
        if (trimmed.length === 0) return;
        const timer = setTimeout(() => {
            const abort = new AbortController();
            abortRef.current = abort;
            api<PersonSearchResult[] | null>(
                `/api/search/people?q=${encodeURIComponent(trimmed)}`,
                { signal: abort.signal },
            )
                .then((results) => {
                    setResultsState({ kind: 'ok', query: trimmed, results: results ?? [] });
                })
                .catch((err) => {
                    if ((err as Error).name === 'AbortError') return;
                    setResultsState({ kind: 'error', query: trimmed });
                });
        }, DEBOUNCE_MS);
        // Cancels both the pending debounce and any in-flight request from the previous query —
        // covers a fast retype, a real query change, and unmount alike.
        return () => {
            clearTimeout(timer);
            abortRef.current?.abort();
            abortRef.current = null;
        };
    }, [query]);

    const view = personSearchView(query, resultsState);

    return (
        <Combobox<PersonSearchResult>
            items={view.items}
            value={null}
            filter={null}
            itemToStringLabel={(result) =>
                result.displayName?.trim() || `@${result.handle}`
            }
            inputValue={query}
            open={open && !view.idle}
            onOpenChange={setOpen}
            onInputValueChange={(next, eventDetails) => {
                // Selecting an item also fires this (Base UI fills the input with its label by
                // default); we reset the field ourselves in onValueChange instead, so ignore it here.
                if (eventDetails.reason === 'item-press') return;
                setQuery(next);
                setOpen(next.trim().length > 0);
                onQueryChange();
            }}
            onValueChange={(result) => {
                if (!result) return;
                setQuery('');
                setOpen(false);
                onSelect(result);
            }}
        >
            <div ref={anchorRef}>
                <ComboboxInput
                    aria-label="Search people"
                    placeholder="Search people"
                    showTrigger={false}
                    showClear
                >
                    <InputGroupAddon align="inline-start">
                        <SearchIcon className="size-4" />
                    </InputGroupAddon>
                </ComboboxInput>
            </div>
            <ComboboxContent anchor={anchorRef}>
                <SearchContent view={view} resultsState={resultsState} />
            </ComboboxContent>
        </Combobox>
    );
}

function SearchContent({
    view,
    resultsState,
}: {
    view: PersonSearchView;
    resultsState: PersonSearchResultsState;
}) {
    if (view.idle) return null;
    if (view.pending) return <SearchLoadingRow />;
    if (resultsState.kind === 'error') return <SearchErrorRow />;
    return (
        <>
            <ComboboxEmpty>No one found.</ComboboxEmpty>
            <ComboboxList>
                {(result: PersonSearchResult) => (
                    <ComboboxItem key={result.did} value={result}>
                        <SearchResultRow result={result} />
                    </ComboboxItem>
                )}
            </ComboboxList>
        </>
    );
}

function SearchResultRow({ result }: { result: PersonSearchResult }) {
    const lines = personRowLines({
        handle: result.handle,
        displayName: result.displayName,
        did: result.did,
        handleAsSecondary: true,
    });
    const avatarSrc = safeHref(result.avatar);

    return (
        <div className="flex min-w-0 flex-1 items-center gap-2.5 py-1">
            <Avatar className="size-8 shrink-0">
                {avatarSrc ? <AvatarImage src={avatarSrc} alt="" /> : null}
                <AvatarFallback>
                    {initialsFromHandle(result.handle, result.did)}
                </AvatarFallback>
            </Avatar>
            <div className="min-w-0 flex-1">
                <div className="flex items-center gap-1.5">
                    <span className="truncate text-sm text-foreground">
                        {lines.primary}
                    </span>
                    {result.inReaderNetwork ? (
                        <Badge className="shrink-0">In reader network</Badge>
                    ) : null}
                </div>
                <p className="truncate text-xs font-light text-muted-foreground">
                    {searchResultHint(result) ?? lines.secondary}
                </p>
            </div>
        </div>
    );
}

function SearchLoadingRow() {
    return (
        <p className="px-2 py-3 text-center text-sm font-light text-muted-foreground">
            Searching…
        </p>
    );
}

function SearchErrorRow() {
    return (
        <p className="px-2 py-3 text-center text-sm font-light text-muted-foreground">
            Couldn’t search right now.
        </p>
    );
}
