import { Link } from 'wouter';

import { EmptyState } from '@/components/status-state';
import { Button } from '@/components/ui/button';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { PATHS } from '@/lib/paths';

export function NotFound() {
    useDocumentTitle('Page not found');
    return (
        <main className="mx-auto w-full max-w-3xl px-5 pt-20 sm:px-8 md:pt-24" id="main">
            <h1 className="sr-only">Page not found</h1>
            <EmptyState
                title="Page not found"
                description="This address doesn’t lead to anything in Morgenblau."
                action={<Button render={<Link href={PATHS.digest} />} nativeButton={false} variant="secondary">Go to the digest</Button>}
            />
        </main>
    );
}
