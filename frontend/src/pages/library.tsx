import { SavedPanel } from '@/components/library/saved-panel';
import { useDocumentTitle } from '@/hooks/use-document-title';

export function Library() {
    useDocumentTitle('Library');

    return (
        <div className="mx-auto w-full max-w-2xl px-4 pt-10 pb-12 sm:px-6">
            <SavedPanel />
        </div>
    );
}
