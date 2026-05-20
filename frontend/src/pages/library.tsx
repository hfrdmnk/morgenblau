import { useDocumentTitle } from '@/hooks/use-document-title';

// Library — curated reading pool (my saves + shared by my network). UI lands later.
export function Library() {
    useDocumentTitle('Library');
    return (
        <main className="flex min-h-full items-center justify-center p-8">
            <p className="text-lg text-muted-foreground">
                Library is coming. Watch this space.
            </p>
        </main>
    );
}
