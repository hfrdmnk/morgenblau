import { useLocation, useParams } from 'wouter';

import { EditSourceSheet } from '@/components/add-source-dialog';
import { SourceHealth } from '@/components/feed-health';
import { SourceDetailPage, SourceEntries, SourceHeader, SourceUnavailable } from '@/components/source-detail';
import { useSources, type FeedSourceDetail, type SourceEntry } from '@/hooks/use-sources';
import { httpURL } from '@/lib/reader';
import { PATHS, sourceHref } from '@/lib/paths';
import { displayURL, feedFacts } from '@/lib/source-display';

export function Source() {
    const { rkey = '' } = useParams<{ rkey: string }>();
    const path = `/api/subscriptions/${encodeURIComponent(rkey)}`;
    const source = useSources<FeedSourceDetail>(path, 15_000);
    const entries = useSources<SourceEntry[]>(`${path}/entries`);
    const [, navigate] = useLocation();

    return (
        <SourceDetailPage>
            {source.status === 'loaded' ? (
                <>
                    <FeedSourceHeader onChanged={source.refresh} onRemoved={() => navigate(PATHS.sources)} source={source.data} />
                    <SourceEntries empty="No posts have arrived from this source yet." from={sourceHref(rkey)} state={entries} title="Posts" />
                </>
            ) : <SourceUnavailable label="this source" state={source} />}
        </SourceDetailPage>
    );
}

function FeedSourceHeader({ source, onChanged, onRemoved }: { source: FeedSourceDetail; onChanged: () => void; onRemoved: () => void }) {
    const site = httpURL(source.siteUrl) || httpURL(source.feedUrl) || source.feedUrl;
    const title = source.title || displayURL(site);
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
