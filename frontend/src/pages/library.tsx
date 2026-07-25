import { NetworkPanel } from '@/components/library/network-panel';
import { SavedPanel } from '@/components/library/saved-panel';
import { SharedPanel } from '@/components/library/shared-panel';
import { Subnav } from '@/components/subnav';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { useSubnavTab } from '@/hooks/use-subnav-tab';

type Subtab = 'saved' | 'shared' | 'network';

const SUBTABS: { id: Subtab; label: string }[] = [
    { id: 'saved', label: 'Saved' },
    { id: 'shared', label: 'Shared' },
    { id: 'network', label: 'Network' },
];

const TAB_IDS: readonly Subtab[] = ['saved', 'shared', 'network'];

export function Library() {
    useDocumentTitle('Library');
    const { active, handleSelect } = useSubnavTab(TAB_IDS, 'saved');

    return (
        <div className="mx-auto w-full max-w-2xl px-4 pt-10 pb-12 sm:px-6">
            <header className="mb-6">
                <Subnav
                    items={SUBTABS}
                    activeId={active}
                    onSelect={handleSelect}
                    ariaLabel="Library section"
                />
            </header>
            {active === 'shared' ? (
                <SharedPanel />
            ) : active === 'network' ? (
                <NetworkPanel />
            ) : (
                <SavedPanel />
            )}
        </div>
    );
}
