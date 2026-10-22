import { useParams } from 'wouter';

import { SourceDetailPage, SourceEntries, SourceHeader, SourceUnavailable } from '@/components/source-detail';
import { SourceMenu } from '@/components/source-menu';
import { useSources, type NewsletterSourceDetail, type SourceEntry } from '@/hooks/use-sources';
import { newsletterSourceHref } from '@/lib/paths';
import { newsletterEditable } from '@/lib/source-edit';
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
                    <NewsletterHeader onChanged={() => {
                        source.refresh();
                        entries.refresh();
                    }} source={source.data} />
                    <SourceEntries empty={source.data.status === 'stopped' ? 'Unsaved issues were removed when this newsletter stopped.' : 'No issues have arrived yet.'} from={newsletterSourceHref(id)} state={entries} title="Issues" />
                </>
            ) : <SourceUnavailable label="this newsletter" state={source} />}
        </SourceDetailPage>
    );
}

function NewsletterHeader({ source, onChanged }: { source: NewsletterSourceDetail; onChanged: () => void }) {
    const title = source.title || source.senderAddress;
    return (
        <SourceHeader
            facts={newsletterFacts(source)}
            menu={<SourceMenu onChanged={onChanged} source={newsletterEditable(source, source.status === 'stopped')} />}
            identity={title === source.senderAddress ? null : <span className="break-all">{source.senderAddress}</span>}
            tags={source.tags}
            title={title}
        />
    );
}
