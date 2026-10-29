import { useDocumentTitle } from '@/hooks/use-document-title';

type PlaceholderProps = { label: string };

export function Placeholder({ label }: PlaceholderProps) {
    useDocumentTitle(label);
    return (
        <main className="page-placeholder" id="main">
            <h1>{label}</h1>
        </main>
    );
}
