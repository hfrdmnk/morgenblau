import { Skeleton } from '@/components/ui/skeleton';

export function CardMasthead({
    eyebrow,
    heading,
    meta,
}: {
    eyebrow: string;
    heading: string;
    meta?: string;
}) {
    return (
        <div className="flex flex-col gap-1 px-6 pt-6 pb-5">
            <p className="text-sm font-light text-muted-foreground">{eyebrow}</p>
            <div className="flex items-baseline justify-between gap-4">
                <h2 className="text-xl font-medium">{heading}</h2>
                {meta ? (
                    <p className="shrink-0 text-sm text-muted-foreground">{meta}</p>
                ) : null}
            </div>
        </div>
    );
}

export function CardMastheadSkeleton() {
    return (
        <div className="flex flex-col gap-1 px-6 pt-6 pb-5">
            <Skeleton className="h-4 w-20" />
            <Skeleton className="h-6 w-2/3" />
        </div>
    );
}
