import { useDocumentTitle } from '@/hooks/use-document-title';

// Per SPEC <daily-digests>: empty-edition placeholder until the digest renderer lands.
export function Consume() {
    useDocumentTitle('Consume');
    return (
        <main className="flex min-h-full items-center justify-center p-8">
            <p className="text-lg text-muted-foreground">
                Nothing new this morning. Enjoy your coffee.
            </p>
        </main>
    );
}
