import { Link } from 'wouter';

import { ArrowRightIcon } from '@/components/icons';

export function BackLink({ href, label }: { href: string; label: string }) {
    return (
        <Link className="inline-flex min-h-10 items-center gap-2 rounded-sm text-sm text-muted-foreground hover:text-foreground" href={href}>
            <ArrowRightIcon className="size-4 rotate-180" />
            {label}
        </Link>
    );
}
