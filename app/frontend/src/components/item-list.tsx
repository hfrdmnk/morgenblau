import type { ReactNode } from 'react';
import { Link } from 'wouter';

import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

export function ItemList({ children, label, ordered = false }: {
    children: ReactNode;
    label?: string;
    ordered?: boolean;
}) {
    const List = ordered ? 'ol' : 'ul';
    return <List aria-label={label} className="space-y-8">{children}</List>;
}

export function ListItem({ title, href, aside, icon, meta, status, note, actions }: {
    title: string;
    href: string;
    aside?: ReactNode;
    icon?: string | null;
    meta?: ReactNode;
    status?: ReactNode;
    note?: ReactNode;
    actions?: ReactNode;
}) {
    return (
        <li className="flex min-w-0 items-center gap-3">
            <div className="min-w-0 flex-1">
                <div className="flex min-w-0 items-baseline gap-3">
                    <Link
                        className="block min-w-0 truncate rounded-sm text-lg font-medium text-foreground hover:text-foreground/75"
                        href={href}
                        title={title}
                    >
                        {title}
                    </Link>
                    {aside ? <span className="shrink-0 text-xs text-muted-foreground">{aside}</span> : null}
                </div>
                {icon || meta ? (
                    <div className="mt-2 flex min-w-0 items-center gap-2.5 text-sm text-subtle-foreground">
                        <SourceIcon src={icon} />
                        <span className="min-w-0 truncate">{meta}</span>
                        {status}
                    </div>
                ) : null}
                {note}
            </div>
            {actions}
        </li>
    );
}

export function SourceIcon({ src, className }: { src?: string | null; className?: string }) {
    if (!src) return null;
    return (
        <img
            alt=""
            className={cn('size-4 shrink-0 rounded-sm object-cover', className)}
            loading="lazy"
            onError={(event) => {
                event.currentTarget.hidden = true;
            }}
            referrerPolicy="no-referrer"
            src={src}
        />
    );
}

export function ListSkeleton({ label }: { label: string }) {
    return (
        <div aria-label={label} className="space-y-8" role="status">
            {[72, 88, 64].map((width) => (
                <div key={width}>
                    <div className="h-6.5 animate-pulse rounded bg-muted motion-reduce:animate-none" style={{ width: `${width}%` }} />
                    <div className="mt-2 h-5 w-2/5 animate-pulse rounded bg-muted motion-reduce:animate-none" />
                </div>
            ))}
        </div>
    );
}

export function ListError({ label, retry }: { label: string; retry: () => void }) {
    return (
        <div className="flex flex-wrap items-center gap-4 text-sm text-muted-foreground" role="alert">
            <p>Couldn’t load {label}.</p>
            <Button onClick={retry} size="sm" variant="secondary">Try again</Button>
        </div>
    );
}
