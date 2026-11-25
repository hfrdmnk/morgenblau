import { useState, type ReactNode } from 'react';
import { toast } from 'sonner';
import { useSearch } from 'wouter/use-browser-location';

import { ItemList, ListError, ListItem } from '@/components/item-list';
import { ArchiveIcon, CopyIcon, ExternalLinkIcon, ReadAgainIcon, TrashIcon } from '@/components/icons';
import { EmptyState } from '@/components/status-state';
import {
    ContextMenu,
    ContextMenuContent,
    ContextMenuItem,
    ContextMenuSeparator,
    ContextMenuTrigger,
} from '@/components/ui/context-menu';
import { PostList } from '@/components/ui/post-list';
import { Skeleton } from '@/components/ui/skeleton';
import { Frame } from '@/design/frame';
import { Text } from '@/design/text';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { useLibraryFiling } from '@/hooks/use-library-filing';
import { useSources } from '@/hooks/use-sources';
import { openInTab } from '@/lib/commands';
import {
    libraryTab,
    queueGroups,
    saveKey,
    savedLabel,
    saveState,
    visibleTags,
    type LibrarySave,
    type SaveState,
} from '@/lib/library';
import { entryHref, libraryHref } from '@/lib/paths';

type Filing = ReturnType<typeof useLibraryFiling>;

export function Library() {
    const tab = libraryTab(useSearch());
    const state = useSources<LibrarySave[]>('/api/saves');
    const filing = useLibraryFiling(state.refresh);
    useDocumentTitle('Library');
    return (
        <Frame as="main" id="main" maxWidth="3xl" paddingX={{ base: 'xl', sm: '3xl' }} width="full">
            <Text as="h1" visuallyHidden>
                {tab === 'archived' ? 'Archive' : 'To read'}
            </Text>
            <Skeleton
                data={state.status === 'loaded' ? state.data : null}
                error={state.status === 'error' ? <ListError label="your Library" retry={state.retry} /> : undefined}
                label="Loading your Library"
                sample={SAMPLE_SAVES}
            >
                {(saves) => {
                    const shown = saves.filter(
                        (save) => !filing.isRemoved(save) && saveState(filing.tagsOf(save)) === tab,
                    );
                    return tab === 'archived' ? (
                        <Archive filing={filing} saves={shown} />
                    ) : (
                        <ToRead filing={filing} saves={shown} />
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

function ToRead({ saves, filing }: { saves: LibrarySave[]; filing: Filing }) {
    if (!saves.length)
        return (
            <EmptyState
                description="Save a post from your digest and it waits here until you have time for it."
                symbol="shelf"
                title="Nothing waiting."
            />
        );
    const groups = queueGroups(saves);
    return (
        <SaveMenu filing={filing} saves={groups.flatMap((group) => group.saves)} tab="queue">
            <Frame gap="7xl">
                {groups.map((group) => (
                    <Frame aria-label={group.title} as="section" gap="2xl" key={group.title}>
                        <Text as="h2" size="sm" tone="subtle" weight="medium">
                            {group.title}
                        </Text>
                        <ItemList>
                            {group.saves.map((save) => (
                                <SaveItem
                                    detail={savedLabel(save.createdAt)}
                                    key={saveKey(save)}
                                    save={save}
                                    tab="queue"
                                />
                            ))}
                        </ItemList>
                    </Frame>
                ))}
            </Frame>
        </SaveMenu>
    );
}

function Archive({ saves, filing }: { saves: LibrarySave[]; filing: Filing }) {
    if (!saves.length)
        return (
            <EmptyState
                description="Archive a post once you’ve read it, and it stays here to come back to."
                symbol="shelf"
                title="The archive is empty."
            />
        );
    return (
        <SaveMenu filing={filing} saves={saves} tab="archived">
            <ItemList>
                {saves.map((save) => (
                    <SaveItem
                        detail={visibleTags(filing.tagsOf(save)).join(', ') || savedLabel(save.createdAt)}
                        key={saveKey(save)}
                        save={save}
                        tab="archived"
                    />
                ))}
            </ItemList>
        </SaveMenu>
    );
}

function SaveMenu({
    saves,
    tab,
    filing,
    children,
}: {
    saves: LibrarySave[];
    tab: SaveState;
    filing: Filing;
    children: ReactNode;
}) {
    const [target, setTarget] = useState<LibrarySave | null>(null);
    return (
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
                {target ? <SaveMenuItems filing={filing} save={target} tab={tab} /> : null}
            </ContextMenuContent>
        </ContextMenu>
    );
}

function SaveMenuItems({ save, tab, filing }: { save: LibrarySave; tab: SaveState; filing: Filing }) {
    return (
        <>
            {tab === 'queue' ? (
                <ContextMenuItem onClick={() => filing.file(save, 'archived')}>
                    <ArchiveIcon />
                    Archive
                </ContextMenuItem>
            ) : (
                <ContextMenuItem onClick={() => filing.file(save, 'queue')}>
                    <ReadAgainIcon />
                    Move to To read
                </ContextMenuItem>
            )}
            <LinkItems save={save} />
            <ContextMenuSeparator />
            <ContextMenuItem onClick={() => filing.remove(save)} tone="destructive">
                <TrashIcon />
                Remove from Library
            </ContextMenuItem>
        </>
    );
}

function LinkItems({ save }: { save: LibrarySave }) {
    if (save.kind === 'newsletter') return null;
    const link = save.targetUrl ?? save.itemUrl;
    return (
        <>
            <ContextMenuItem onClick={() => openInTab(link)}>
                <ExternalLinkIcon />
                Open original
            </ContextMenuItem>
            <ContextMenuItem onClick={() => void navigator.clipboard.writeText(link).then(() => toast('Link copied'))}>
                <CopyIcon />
                Copy link
            </ContextMenuItem>
        </>
    );
}

function SaveItem({ save, tab, detail }: { save: LibrarySave; tab: SaveState; detail: string }) {
    const meta = [sourceLabel(save), save.minutes ? `${save.minutes} min` : null, detail].filter(Boolean).join(' · ');
    return (
        <ListItem
            href={saveHref(save, tab)}
            icon={save.source?.faviconUrl}
            meta={meta}
            title={save.title || 'Untitled'}
        />
    );
}

function saveHref(save: LibrarySave, tab: SaveState): string {
    if (save.entrySlug) return entryHref(save.entrySlug, libraryHref(tab));
    return save.kind === 'newsletter' ? libraryHref() : save.itemUrl;
}

function sourceLabel(save: LibrarySave): string {
    if (save.source?.title) return save.source.title;
    return save.kind === 'newsletter' ? 'Newsletter' : hostname(save.itemUrl);
}

function hostname(url: string): string {
    try {
        return new URL(url).hostname.replace(/^www\./, '');
    } catch {
        return url;
    }
}
