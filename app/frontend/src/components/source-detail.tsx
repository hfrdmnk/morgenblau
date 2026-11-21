import type { ReactNode } from 'react';

import { BackLink } from '@/components/back-link';
import { ItemList, ListError, ListItem } from '@/components/item-list';
import { PostList } from '@/components/ui/post-list';
import { SourceIcon } from '@/components/ui/source-icon';
import { Skeleton } from '@/components/ui/skeleton';
import { PageSection } from '@/components/page-section';
import { Badge } from '@/components/ui/badge';
import { Frame } from '@/design/frame';
import { Text } from '@/design/text';
import { GutterMark, PrimaryMark } from '@/components/ui/source-mark';
import { TooltipProvider } from '@/components/ui/tooltip';
import { useSources, type SourceEntry } from '@/hooks/use-sources';
import { entryHref, PATHS } from '@/lib/paths';
import { formatDate } from '@/lib/source-display';

type Loadable<T> = ReturnType<typeof useSources<T>>;

export function SourceDetailPage({ children }: { children: ReactNode }) {
    return (
        <Frame
            as="main"
            gap={{ base: '4xl', md: '5xl' }}
            id="main"
            maxWidth="3xl"
            paddingX={{ base: 'xl', sm: '3xl' }}
            width="full"
        >
            <Frame as="nav" align="start" aria-label="Source">
                <BackLink href={PATHS.sources} label="Sources" />
            </Frame>
            <Frame>{children}</Frame>
        </Frame>
    );
}

export function SourceHeader({
    title,
    primary,
    icon,
    identity,
    facts,
    tags = [],
    health,
    menu,
}: {
    title: string;
    primary: boolean;
    icon?: string | null;
    identity?: ReactNode;
    facts: string[];
    tags?: string[];
    health?: ReactNode;
    menu: ReactNode;
}) {
    return (
        // Each line keeps its own distance from the one above: identity and health stand apart, facts tuck under.
        <TooltipProvider>
            <Frame as="header" gap="l" paddingBottom="5xl">
                <Frame>
                    <Frame align="center" direction="row" gap="m" minWidth={0} position={{ sm: 'relative' }}>
                        {primary ? (
                            <GutterMark offset="title">
                                <PrimaryMark size="xl" />
                            </GutterMark>
                        ) : null}
                        <SourceIcon size="2xl" src={icon} />
                        <Frame grow minWidth={0}>
                            <Text as="h1" break="words" size="3xl" tracking="tight" weight="medium" wrap="balance">
                                {title}
                            </Text>
                        </Frame>
                        {menu}
                    </Frame>
                    {identity ? (
                        <Frame minWidth={0} paddingTop="m">
                            <Text as="span" size="sm" tone="muted">
                                <Frame align="center" as="span" direction="row" gap="s" minWidth={0}>
                                    {identity}
                                </Frame>
                            </Text>
                        </Frame>
                    ) : null}
                    <Frame paddingTop="2xs">
                        <Text size="sm" tone="subtle">
                            {facts.join(' · ')}
                        </Text>
                    </Frame>
                    {health}
                </Frame>
                <SourceTags tags={tags} />
            </Frame>
        </TooltipProvider>
    );
}

function SourceTags({ tags }: { tags: string[] }) {
    if (!tags.length) return null;
    return (
        <Frame as="ul" aria-label="Tags" direction="row" gap="s" wrap>
            {tags.map((tag) => (
                <Frame as="li" key={tag}>
                    <Badge variant="tag">{tag}</Badge>
                </Frame>
            ))}
        </Frame>
    );
}

export function SourceEntries({
    title,
    ...props
}: {
    title: string;
    from: string;
    state: Loadable<SourceEntry[]>;
    empty: string;
}) {
    return (
        <PageSection title={title}>
            <EntriesBody label={title.toLowerCase()} {...props} />
        </PageSection>
    );
}

function EntriesBody({
    label,
    from,
    state,
    empty,
}: {
    label: string;
    from: string;
    state: Loadable<SourceEntry[]>;
    empty: string;
}) {
    return (
        <Skeleton
            data={state.status === 'loaded' ? state.data : null}
            error={state.status === 'error' ? <ListError label={label} retry={state.retry} /> : undefined}
            label={`Loading ${label}`}
            sample={SAMPLE_ENTRIES}
        >
            {(entries) =>
                entries.length ? (
                    <PostList>
                        <ItemList>
                            {entries.map((entry) => (
                                <ListItem
                                    href={entryHref(entry.entrySlug, from)}
                                    key={entry.entrySlug}
                                    meta={formatDate(entry.publishedAt)}
                                    title={entry.title || 'Untitled'}
                                />
                            ))}
                        </ItemList>
                    </PostList>
                ) : (
                    <Text size="sm" tone="muted">
                        {empty}
                    </Text>
                )
            }
        </Skeleton>
    );
}

const SAMPLE_ENTRIES: SourceEntry[] = [
    'A sample headline of a typical length',
    'A shorter sample headline',
    'Another sample headline that runs a little longer',
].map((title, index) => ({
    id: index,
    entrySlug: `${index}`,
    title,
    contentType: 'blogpost',
    publishedAt: '2026-01-01T00:00:00Z',
}));

export function SourceUnavailable({
    state,
    label,
}: {
    state: Extract<Loadable<unknown>, { status: 'error' }>;
    label: string;
}) {
    if (state.notFound) {
        return (
            <Frame gap="m" role="alert">
                <Text as="h1" size="3xl" tracking="tight" weight="medium">
                    Source unavailable
                </Text>
                <Text tone="muted">It may have been removed from your sources.</Text>
            </Frame>
        );
    }
    return <ListError label={label} retry={state.retry} />;
}
