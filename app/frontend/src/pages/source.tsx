import { Skeleton } from 'skullet/react';
import { useLocation, useParams } from 'wouter';

import { EditSourceSheet } from '@/components/add-source-dialog';
import { SourceHealth } from '@/components/feed-health';
import { SourceDetailPage, SourceEntries, SourceHeader, SourceUnavailable } from '@/components/source-detail';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { useFeedSourceCommands } from '@/hooks/use-source-commands';
import { useSources, type FeedSourceDetail, type SourceEntry } from '@/hooks/use-sources';
import { httpURL } from '@/lib/reader';
import { PATHS, sourceHref } from '@/lib/paths';
import { displayURL, feedFacts, feedSite, feedTitle } from '@/lib/source-display';

export function Source() {
    const { rkey = '' } = useParams<{ rkey: string }>();
    const path = `/api/subscriptions/${encodeURIComponent(rkey)}`;
    const source = useSources<FeedSourceDetail>(path, 15_000);
    const entries = useSources<SourceEntry[]>(`${path}/entries`);
    const [, navigate] = useLocation();
    const loaded = source.status === 'loaded' ? source.data : null;
    useDocumentTitle(loaded ? feedTitle(loaded) : 'Source');
    useFeedSourceCommands(loaded, entries);

    return (
        <SourceDetailPage>
            <Skeleton
                data={loaded}
                error={source.status === 'error' ? <SourceUnavailable label="this source" state={source} /> : undefined}
                id="source"
                label="Loading this source"
                sample={SAMPLE_SOURCE}
            >
                {(data) => (
                    <>
                        <FeedSourceHeader onChanged={source.refresh} onRemoved={() => navigate(PATHS.sources)} source={data} />
                        <SourceEntries empty="No posts have arrived from this source yet." from={sourceHref(rkey)} state={entries} title="Posts" />
                    </>
                )}
            </Skeleton>
        </SourceDetailPage>
    );
}

const SAMPLE_SOURCE: FeedSourceDetail = {
    rkey: 'sample', title: 'Example Publication', feedUrl: 'https://example.com/feed.xml', siteUrl: 'https://example.com',
    primary: false, fetchStatus: 'ready', frequency: 'weekly', totalEntries: 12, lastPublishedAt: '2026-01-01T00:00:00Z',
};

function FeedSourceHeader({ source, onChanged, onRemoved }: { source: FeedSourceDetail; onChanged: () => void; onRemoved: () => void }) {
    const site = feedSite(source);
    const title = feedTitle(source);
    return (
        <SourceHeader
            facts={feedFacts(source)}
            health={<SourceHealth source={source} />}
            icon={source.faviconUrl}
            identity={<SiteLink url={site} />}
            menu={<EditSourceSheet onChanged={onChanged} onRemoved={onRemoved} source={source} />}
            tags={source.tags}
            title={title}
        />
    );
}

function SiteLink({ url }: { url: string }) {
    if (!httpURL(url)) return <span className="break-all">{displayURL(url)}</span>;
    return (
        <a className="break-all underline decoration-border underline-offset-4 hover:text-foreground" href={url} rel="noopener noreferrer" target="_blank">
            {displayURL(url)}
        </a>
    );
}
