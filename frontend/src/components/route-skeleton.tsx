import { Skeleton } from '@/components/ui/skeleton';

// Suspense fallback while a route chunk fetches; kept generic since it can sit under any page.
export function RouteSkeleton() {
    return (
        <div className="flex justify-center px-4 py-8">
            <article
                aria-busy
                aria-label="Loading page"
                className="w-full max-w-2xl overflow-hidden rounded-xl bg-card p-6 shadow-card"
            >
                <div className="flex flex-col gap-3">
                    <Skeleton className="h-5 w-1/3" />
                    <Skeleton className="h-3 w-full" />
                    <Skeleton className="h-3 w-5/6" />
                    <Skeleton className="h-3 w-2/3" />
                </div>
            </article>
        </div>
    );
}
