import { useDocumentTitle } from '@/hooks/use-document-title';

// Coming soon — Discover mode (SPEC <modes>) lands after v1.
export function Discover() {
    useDocumentTitle('Discover');
    return (
        <main className="flex min-h-full items-center justify-center p-8">
            <p className="text-lg text-muted-foreground">
                Discover is coming. Watch this space.
            </p>
        </main>
    );
}
