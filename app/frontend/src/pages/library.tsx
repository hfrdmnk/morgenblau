import { useRef, useState, type ReactNode, type RefObject } from 'react';
import { toast } from 'sonner';
import { useLocation } from 'wouter';
import { useHistoryState, useSearch } from 'wouter/use-browser-location';

import { CopyIcon, ExternalLinkIcon, SearchIcon, TrashIcon } from '@/components/icons';
import { ItemList, ListError, ListItem } from '@/components/item-list';
import { EmptyState } from '@/components/status-state';
import { Button } from '@/components/ui/button';
import {
    ContextMenu,
    ContextMenuContent,
    ContextMenuItem,
    ContextMenuSeparator,
    ContextMenuTrigger,
} from '@/components/ui/context-menu';
import { Input } from '@/components/ui/input';
import { Kbd } from '@/components/ui/kbd';
import { PostList } from '@/components/ui/post-list';
import { Skeleton } from '@/components/ui/skeleton';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { Frame } from '@/design/frame';
import { Text } from '@/design/text';
import type { AppNavigationState } from '@/hooks/use-app-location';
import { useCommands } from '@/hooks/use-commands';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { useSaveRemoval } from '@/hooks/use-save-removal';
import { useShortcuts } from '@/hooks/use-shortcuts';
import { useSources } from '@/hooks/use-sources';
import { openInTab } from '@/lib/commands';
import {
    groupSaves,
    librarySort,
    libraryView,
    matchesQuery,
    RECENT_SIZE,
    saveKey,
    savedLabel,
    saveSourceLabel,
    saveTitle,
    type LibrarySave,
    type LibrarySort,
} from '@/lib/library';
import { libraryHref, saveHref } from '@/lib/paths';

type Removal = ReturnType<typeof useSaveRemoval>;

const HEADINGS = { recent: 'Recent saves', everything: 'Everything in your Library' } as const;

export function Library() {
    const search = useSearch();
    const view = libraryView(search);
    const sort = librarySort(search);
    const query = new URLSearchParams(search).get('q') ?? '';
    const state = useSources<LibrarySave[]>('/api/saves');
    const removal = useSaveRemoval(state.refresh);
    useDocumentTitle('Library');
    return (
        <Frame as="main" id="main" maxWidth="3xl" paddingX={{ base: 'xl', sm: '3xl' }} width="full">
            <Text as="h1" visuallyHidden>
                {HEADINGS[view]}
            </Text>
            <Skeleton
                data={state.status === 'loaded' ? state.data : null}
                error={state.status === 'error' ? <ListError label="your Library" retry={state.retry} /> : undefined}
                label="Loading your Library"
                sample={SAMPLE_SAVES}
            >
                {(saves) => {
                    const shown = saves.filter((save) => !removal.isRemoved(save));
                    return view === 'everything' ? (
                        <Everything initialQuery={query} removal={removal} saves={shown} sort={sort} />
                    ) : (
                        <Recent removal={removal} saves={shown.slice(0, RECENT_SIZE)} />
                    );
                }}
            </Skeleton>
        </Frame>
    );
}

const SAMPLE_SAVES: LibrarySave[] = [
    'A sample headline that runs about as long as most do',
    'A shorter sample headline',
    'Another sample headline of a typical length',
    'A sample headline',
].map((title, index) => ({
    rkey: `${index}`,
    itemUrl: 'https://example.com/post',
    createdAt: '2026-01-01T00:00:00Z',
    title,
    entrySlug: `${index}`,
    minutes: 4,
    source: { title: 'Example Publication' },
}));

function Recent({ saves, removal }: { saves: LibrarySave[]; removal: Removal }) {
    const [, navigate] = useLocation();
    const searchEverything = () => navigate(libraryHref('everything'), { state: { openSearch: true } });
    useShortcuts({ '/': searchEverything, 'Shift+/': searchEverything });
    if (!saves.length)
        return (
            <EmptyState
                description="Save a post from your digest and your newest saves wait here."
                title="Nothing saved yet."
            />
        );
    return (
        <SaveList removal={removal} saves={saves}>
            <ItemList>
                {saves.map((save) => (
                    <SaveItem
                        detail={savedLabel(save.createdAt)}
                        key={saveKey(save)}
                        from={libraryHref()}
                        removal={removal}
                        save={save}
                    />
                ))}
            </ItemList>
        </SaveList>
    );
}

function Everything({
    saves,
    sort,
    initialQuery,
    removal,
}: {
    saves: LibrarySave[];
    sort: LibrarySort;
    initialQuery: string;
    removal: Removal;
}) {
    const { query, open, focus, input, toggle, change, close, flip } = useLibrarySearch(sort, initialQuery);
    useSortCommand(sort, query);
    if (!saves.length)
        return (
            <EmptyState
                description="Everything you save from your digest stays here, newest first."
                title="Nothing saved yet."
            />
        );
    const found = saves.filter((save) => matchesQuery(save, query));
    const groups = groupSaves(found, sort);
    return (
        <Frame gap="4xl">
            <Frame gap="l">
                <Frame align="center" direction="row" gap="l" justify="between" minHeight="control">
                    <Text aria-live="polite" as="span" numeric="tabular" size="sm" tone="subtle">
                        {countLabel(found.length, saves.length, query)}
                    </Text>
                    <Frame align="center" direction="row" gap="l">
                        <SortChoices query={query} sort={sort} />
                        <RowAction
                            buttonRef={toggle}
                            keys="/"
                            label="Search your Library"
                            onClick={flip}
                            pressed={open}
                        >
                            <SearchIcon />
                        </RowAction>
                    </Frame>
                </Frame>
                {open ? (
                    <SearchField autoFocus={focus} inputRef={input} onChange={change} onClose={close} query={query} />
                ) : null}
            </Frame>
            {found.length ? (
                <SaveList removal={removal} saves={groups.flatMap((group) => group.saves)}>
                    <Frame gap="7xl">
                        {groups.map((group) => (
                            <Frame aria-label={group.title} as="section" gap="2xl" key={group.title}>
                                <Text as="h2" size="sm" tone="subtle" weight="medium">
                                    {group.title}
                                </Text>
                                <ItemList>
                                    {group.saves.map((save) => (
                                        <SaveItem
                                            detail={sort === 'newest' ? undefined : savedLabel(save.createdAt)}
                                            from={libraryHref('everything', { sort, query })}
                                            key={saveKey(save)}
                                            removal={removal}
                                            save={save}
                                        />
                                    ))}
                                </ItemList>
                            </Frame>
                        ))}
                    </Frame>
                </SaveList>
            ) : (
                <EmptyState
                    description="Search looks in titles, site names and tags; #tag finds only that tag."
                    title={`Nothing matches “${query.trim()}”.`}
                />
            )}
        </Frame>
    );
}

function useLibrarySearch(sort: LibrarySort, initialQuery: string) {
    const arrived = useHistoryState<AppNavigationState | null>()?.openSearch === true;
    const [query, setQuery] = useState(initialQuery);
    const [open, setOpen] = useState(arrived || initialQuery !== '');
    const [focus, setFocus] = useState(arrived);
    const [, navigate] = useLocation();
    const input = useRef<HTMLInputElement>(null);
    const toggle = useRef<HTMLButtonElement>(null);
    const change = (next: string) => {
        setQuery(next);
        navigate(libraryHref('everything', { sort, query: next }), { replace: true });
    };
    const show = () => {
        setFocus(true);
        setOpen(true);
        input.current?.focus();
    };
    const close = () => {
        change('');
        setOpen(false);
        toggle.current?.focus();
    };
    useShortcuts({ '/': show, 'Shift+/': show });
    return { query, open, focus, input, toggle, change, close, flip: open ? close : show };
}

function SearchField({
    query,
    autoFocus,
    inputRef,
    onChange,
    onClose,
}: {
    query: string;
    autoFocus: boolean;
    inputRef: RefObject<HTMLInputElement | null>;
    onChange: (query: string) => void;
    onClose: () => void;
}) {
    return (
        <Input
            aria-label="Search your Library"
            autoFocus={autoFocus}
            onChange={(event) => onChange(event.target.value)}
            onKeyDown={(event) => {
                if (event.key !== 'Escape') return;
                event.preventDefault();
                onClose();
            }}
            placeholder="Search titles, sites and #tags"
            ref={inputRef}
            spellCheck={false}
            value={query}
        />
    );
}

function useSortCommand(sort: LibrarySort, query: string) {
    const [, navigate] = useLocation();
    useCommands([
        {
            id: 'library:sort',
            group: 'Everything',
            label: 'Sort by…',
            detail: SORTS.find((choice) => choice.sort === sort)?.label,
            keywords: ['order', 'newest', 'title', 'site'],
            step: {
                kind: 'choice',
                title: 'Sort by',
                placeholder: 'Choose an order',
                current: sort,
                options: SORTS.map((choice) => ({ value: choice.sort, label: choice.label })),
                submit: (next) =>
                    navigate(libraryHref('everything', { sort: librarySort(`sort=${next}`), query }), {
                        replace: true,
                    }),
            },
        },
    ]);
}

function countLabel(found: number, total: number, query: string) {
    if (query.trim()) return `${found} of ${total} saves`;
    return total === 1 ? '1 save' : `${total} saves`;
}

const SORTS: Array<{ sort: LibrarySort; label: string }> = [
    { sort: 'newest', label: 'Newest' },
    { sort: 'title', label: 'Title' },
    { sort: 'site', label: 'Site' },
];

function SortChoices({ sort, query }: { sort: LibrarySort; query: string }) {
    const [, navigate] = useLocation();
    const choose = (next: LibrarySort) => navigate(libraryHref('everything', { sort: next, query }), { replace: true });
    return (
        <Frame align="center" aria-label="Sort" direction="row" gap="l" role="group">
            {SORTS.map((choice) => (
                <Button
                    aria-pressed={sort === choice.sort}
                    key={choice.sort}
                    onClick={() => choose(choice.sort)}
                    size="bare"
                    variant="nav"
                >
                    {choice.label}
                </Button>
            ))}
        </Frame>
    );
}

function linkOf(save: LibrarySave): string | null {
    return save.kind === 'newsletter' ? null : (save.targetUrl ?? save.itemUrl);
}

function copyLink(link: string) {
    void navigator.clipboard.writeText(link).then(
        () => toast('Link copied'),
        () => toast.error('Couldn’t copy the link'),
    );
}

function selectedSave(saves: LibrarySave[]): LibrarySave | undefined {
    const rows = [...document.querySelectorAll('[data-slot=post-list] [data-post]')];
    const row = document.querySelector('[data-slot=post-list] [data-post][data-selected]');
    return row ? saves[rows.indexOf(row)] : undefined;
}

function SaveList({ saves, removal, children }: { saves: LibrarySave[]; removal: Removal; children: ReactNode }) {
    const [target, setTarget] = useState<LibrarySave | null>(null);
    const onSelected = (run: (save: LibrarySave) => void) => () => {
        const save = selectedSave(saves);
        if (save) run(save);
    };
    const remove = onSelected(removal.remove);
    useShortcuts({
        c: onSelected((save) => {
            const link = linkOf(save);
            if (link) copyLink(link);
        }),
        Backspace: remove,
        Delete: remove,
    });
    return (
        <TooltipProvider>
            <ContextMenu>
                <ContextMenuTrigger
                    onContextMenu={(event) => {
                        const rows = [...event.currentTarget.querySelectorAll('[data-post]')];
                        const row = event.target instanceof Element ? event.target.closest('[data-post]') : null;
                        const save = row ? saves[rows.indexOf(row)] : undefined;
                        if (!save) return event.preventBaseUIHandler();
                        setTarget(save);
                    }}
                >
                    <PostList>{children}</PostList>
                </ContextMenuTrigger>
                <ContextMenuContent>
                    {target ? <SaveMenuItems removal={removal} save={target} /> : null}
                </ContextMenuContent>
            </ContextMenu>
        </TooltipProvider>
    );
}

function SaveMenuItems({ save, removal }: { save: LibrarySave; removal: Removal }) {
    const link = linkOf(save);
    return (
        <>
            {link ? (
                <>
                    <ContextMenuItem onClick={() => openInTab(link)}>
                        <ExternalLinkIcon />
                        Open original
                    </ContextMenuItem>
                    <ContextMenuItem onClick={() => copyLink(link)}>
                        <CopyIcon />
                        Copy link
                    </ContextMenuItem>
                    <ContextMenuSeparator />
                </>
            ) : null}
            <ContextMenuItem onClick={() => removal.remove(save)} tone="destructive">
                <TrashIcon />
                Remove from Library
            </ContextMenuItem>
        </>
    );
}

function SaveItem({
    save,
    from,
    detail,
    removal,
}: {
    save: LibrarySave;
    from: string;
    detail?: string;
    removal: Removal;
}) {
    const meta = [saveSourceLabel(save), save.minutes ? `${save.minutes} min` : null, detail]
        .filter(Boolean)
        .join(' · ');
    const link = linkOf(save);
    return (
        <ListItem
            href={saveHref(save, from)}
            icon={save.source?.faviconUrl}
            meta={meta}
            rowActions={
                <>
                    {link ? (
                        <RowAction keys="C" label="Copy link" onClick={() => copyLink(link)}>
                            <CopyIcon />
                        </RowAction>
                    ) : null}
                    <RowAction keys="⌫" label="Remove from Library" onClick={() => removal.remove(save)}>
                        <TrashIcon />
                    </RowAction>
                </>
            }
            title={saveTitle(save)}
        />
    );
}

function RowAction({
    label,
    keys,
    onClick,
    pressed,
    buttonRef,
    children,
}: {
    label: string;
    keys: string;
    onClick: () => void;
    pressed?: boolean;
    buttonRef?: RefObject<HTMLButtonElement | null>;
    children: ReactNode;
}) {
    return (
        <Tooltip>
            <TooltipTrigger
                render={
                    <Button
                        aria-label={label}
                        aria-pressed={pressed}
                        onClick={onClick}
                        ref={buttonRef}
                        size="icon-sm"
                        variant="quiet"
                    >
                        {children}
                    </Button>
                }
            />
            <TooltipContent>
                <Frame align="center" as="span" direction="row" gap="s">
                    {label}
                    <Kbd>{keys}</Kbd>
                </Frame>
            </TooltipContent>
        </Tooltip>
    );
}
