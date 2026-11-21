import { useRef, type ReactNode, type RefObject } from 'react';

import { EditNewsletterSheet, EditSourceSheet } from '@/components/add-source-dialog';
import { FeedHealth, GeneratedNote } from '@/components/feed-health';
import { ItemList, ListError, ListItem } from '@/components/item-list';
import { Skeleton } from '@/components/ui/skeleton';
import { Link } from '@/components/ui/link';
import { PageSection } from '@/components/page-section';
import { Frame } from '@/design/frame';
import { Text } from '@/design/text';
import { GeneratedMark, PrimaryMark } from '@/components/ui/source-mark';
import { TooltipProvider } from '@/components/ui/tooltip';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { useSources, type FeedSource, type NewsletterSource } from '@/hooks/use-sources';
import { newsletterSourceHref, PATHS, sourceHref } from '@/lib/paths';
import { displayURL, groupByInitial } from '@/lib/source-display';

type RemovedFocus = RefObject<HTMLHeadingElement | null>;

export function Sources() {
    const heading = useRef<HTMLHeadingElement>(null);
    useDocumentTitle('Sources');
    return (
        <Frame
            as="main"
            aria-label="Sources"
            gap="5xl"
            id="main"
            maxWidth="3xl"
            paddingX={{ base: 'xl', sm: '3xl' }}
            width="full"
        >
            <Text as="h1" visuallyHidden ref={heading} tabIndex={-1}>
                Sources
            </Text>
            <TooltipProvider>
                <FeedList removedFocus={heading} />
                <NewsletterList removedFocus={heading} />
            </TooltipProvider>
        </Frame>
    );
}

const SAMPLE_FEEDS: FeedSource[] = [
    'Example Publication',
    'Another Example Journal',
    'Sample Weekly',
    'Example Notes',
].map((title, index) => ({
    rkey: `${index}`,
    title,
    feedUrl: 'https://example.com/feed.xml',
    siteUrl: 'https://example.com',
    primary: index === 0,
    fetchStatus: 'ready',
}));

const SAMPLE_NEWSLETTERS: NewsletterSource[] = ['Example Newsletter', 'Sample Dispatch'].map((title, index) => ({
    id: `${index}`,
    title,
    senderAddress: 'hello@example.com',
    primary: false,
    canUnsubscribe: false,
    tags: [],
}));

function FeedList({ removedFocus }: { removedFocus: RemovedFocus }) {
    const state = useSources<FeedSource[]>('/api/subscriptions', 15_000);
    return (
        <PageSection title="Feeds">
            <Skeleton
                data={state.status === 'loaded' ? state.data : null}
                error={state.status === 'error' ? <ListError label="feeds" retry={state.retry} /> : undefined}
                label="Loading feeds"
                sample={SAMPLE_FEEDS}
            >
                {(feeds) =>
                    feeds.length ? (
                        <LetterGroups
                            sources={feeds}
                            label={(source) => sourceLabel(source.title, source.siteUrl || source.feedUrl)}
                            row={(source) => (
                                <SourceRow
                                    key={source.rkey}
                                    href={sourceHref(source.rkey)}
                                    title={source.title}
                                    identity={source.siteUrl || source.feedUrl}
                                    favicon={source.faviconUrl}
                                    primary={source.primary}
                                    generated={Boolean(source.generated)}
                                    health={
                                        <FeedHealth
                                            labelled
                                            source={source}
                                            waitingChannels={waitingOnYouTube(feeds)}
                                        />
                                    }
                                    note={<GeneratedNote source={source} />}
                                    menu={
                                        <EditSourceSheet
                                            onChanged={state.refresh}
                                            removedFocus={removedFocus}
                                            source={source}
                                        />
                                    }
                                />
                            )}
                        />
                    ) : (
                        <Text size="sm" tone="muted">
                            No feeds yet.{' '}
                            <Link href={PATHS.import} variant="underline">
                                Import feeds
                            </Link>{' '}
                            to get started.
                        </Text>
                    )
                }
            </Skeleton>
        </PageSection>
    );
}

function NewsletterList({ removedFocus }: { removedFocus: RemovedFocus }) {
    const state = useSources<{ active: NewsletterSource[]; stopped: NewsletterSource[] }>('/api/newsletters');
    return (
        <>
            <PageSection title="Newsletters">
                <Skeleton
                    data={state.status === 'loaded' ? state.data.active : null}
                    error={state.status === 'error' ? <ListError label="newsletters" retry={state.retry} /> : undefined}
                    label="Loading newsletters"
                    sample={SAMPLE_NEWSLETTERS}
                >
                    {(active) =>
                        active.length ? (
                            <NewsletterRows onChanged={state.refresh} removedFocus={removedFocus} sources={active} />
                        ) : (
                            <Text size="sm" tone="muted">
                                Newsletters appear here when mail arrives at your private address.
                            </Text>
                        )
                    }
                </Skeleton>
            </PageSection>
            {state.status === 'loaded' ? (
                <StoppedNewsletters
                    onChanged={state.refresh}
                    removedFocus={removedFocus}
                    sources={state.data.stopped}
                />
            ) : null}
        </>
    );
}

function StoppedNewsletters({
    sources,
    onChanged,
    removedFocus,
}: {
    sources: NewsletterSource[];
    onChanged: () => void;
    removedFocus: RemovedFocus;
}) {
    if (!sources.length) return null;
    return (
        <PageSection title="Stopped newsletters">
            <NewsletterRows onChanged={onChanged} removedFocus={removedFocus} sources={sources} stopped />
        </PageSection>
    );
}

function NewsletterRows({
    sources,
    stopped = false,
    onChanged,
    removedFocus,
}: {
    sources: NewsletterSource[];
    stopped?: boolean;
    onChanged: () => void;
    removedFocus: RemovedFocus;
}) {
    return (
        <LetterGroups
            sources={sources}
            label={(source) => sourceLabel(source.title, source.senderAddress)}
            row={(source) => (
                <SourceRow
                    key={source.id}
                    href={newsletterSourceHref(source.id)}
                    title={source.title}
                    identity={source.senderAddress}
                    primary={!stopped && source.primary}
                    menu={
                        <EditNewsletterSheet
                            onChanged={onChanged}
                            removedFocus={removedFocus}
                            source={source}
                            stopped={stopped}
                        />
                    }
                />
            )}
        />
    );
}

function LetterGroups<T>({
    sources,
    label,
    row,
}: {
    sources: T[];
    label: (source: T) => string;
    row: (source: T) => ReactNode;
}) {
    return (
        <Frame gap="5xl">
            {groupByInitial(sources, label).map(({ initial, items }) => (
                <Frame key={initial} gap="xl">
                    <Text as="h3" size="sm" weight="medium" tone="subtle">
                        {initial}
                    </Text>
                    <ItemList>{items.map(row)}</ItemList>
                </Frame>
            ))}
        </Frame>
    );
}

function waitingOnYouTube(feeds: FeedSource[]) {
    return feeds.filter((feed) => feed.youtubeOutage).length;
}

function sourceLabel(title: string | undefined, identity: string): string {
    return title || displayURL(identity);
}

function SourceRow({
    href,
    title,
    identity,
    favicon,
    primary,
    generated = false,
    health,
    note,
    menu,
}: {
    href: string;
    title?: string;
    identity: string;
    favicon?: string;
    primary: boolean;
    generated?: boolean;
    health?: ReactNode;
    note?: ReactNode;
    menu: ReactNode;
}) {
    return (
        <ListItem
            href={href}
            title={sourceLabel(title, identity)}
            icon={favicon}
            mark={primary ? <PrimaryMark size="sm" /> : undefined}
            note={note}
            meta={displayURL(identity)}
            status={
                <>
                    {generated ? <GeneratedMark size="sm" tucked /> : null}
                    {health}
                </>
            }
            actions={menu}
        />
    );
}
