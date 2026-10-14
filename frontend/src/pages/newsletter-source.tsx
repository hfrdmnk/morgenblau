import { useParams } from 'wouter';

import { SourceDetailPage, SourceEntries, SourceHeader, SourceUnavailable } from '@/components/source-detail';
import { useSources, type NewsletterSourceDetail, type SourceEntry } from '@/hooks/use-sources';
import { newsletterSourceHref } from '@/lib/paths';
import { newsletterFacts } from '@/lib/source-display';

export function NewsletterSource() {
    const { id = '' } = useParams<{ id: string }>();
    const path = `/api/newsletters/${encodeURIComponent(id)}`;
    const source = useSources<NewsletterSourceDetail>(path);
    const entries = useSources<SourceEntry[]>(`${path}/entries`);

    return (
        <SourceDetailPage>
            {source.status === 'loaded' ? (
                <>
                    <NewsletterHeader source={source.data} />
                    <SourceEntries empty={source.data.status === 'stopped' ? 'Unsaved issues were removed when this newsletter stopped.' : 'No issues have arrived yet.'} from={newsletterSourceHref(id)} state={entries} title="Issues" />
                </>
            ) : <SourceUnavailable label="this newsletter" state={source} />}
        </SourceDetailPage>
    );
}

function NewsletterHeader({ source }: { source: NewsletterSourceDetail }) {
    const title = source.title || source.senderAddress;
    return (
        <SourceHeader
            facts={newsletterFacts(source)}
            identity={title === source.senderAddress ? null : <span className="break-all">{source.senderAddress}</span>}
            tags={source.tags}
            title={title}
        />
    );
}
