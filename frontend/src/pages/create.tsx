import { useDocumentTitle } from '@/hooks/use-document-title';

// Coming soon — Create mode (SPEC <modes>) lands after v1.
export function Create() {
    useDocumentTitle('Create');
    return (
        <main className="flex min-h-full items-center justify-center p-8">
            <p className="text-lg text-muted-foreground">
                Create is coming. Watch this space.
            </p>
        </main>
    );
}
