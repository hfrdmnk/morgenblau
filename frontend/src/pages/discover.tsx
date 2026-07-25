import { PeoplePanel } from '@/components/discover/people-panel';
import { SourcesPanel } from '@/components/discover/sources-panel';
import { Subnav } from '@/components/subnav';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { useSubnavTab } from '@/hooks/use-subnav-tab';

type Subtab = 'sources' | 'people';

const SUBTABS: { id: Subtab; label: string }[] = [
    { id: 'sources', label: 'Sources' },
    { id: 'people', label: 'People' },
];

const TAB_IDS: readonly Subtab[] = ['sources', 'people'];

export function Discover() {
    useDocumentTitle('Discover');
    const { active, handleSelect } = useSubnavTab(TAB_IDS, 'sources');

    return (
        <div className="mx-auto w-full max-w-2xl px-4 pt-10 pb-12 sm:px-6">
            <header className="mb-6">
                <Subnav
                    items={SUBTABS}
                    activeId={active}
                    onSelect={handleSelect}
                    ariaLabel="Discover section"
                />
            </header>
            {active === 'people' ? <PeoplePanel /> : <SourcesPanel />}
        </div>
    );
}
