import { Link } from 'wouter';

import { ArrowRightIcon } from '@/components/icons';

export function BackLink({ href, label, keyShortcut }: { href: string; label: string; keyShortcut?: string }) {
    return (
        <Link aria-keyshortcuts={keyShortcut} className="inline-flex min-h-10 items-center gap-2 rounded-sm text-sm text-muted-foreground hover:text-foreground" href={href}>
            <ArrowRightIcon className="size-4 rotate-180" />
            {label}
        </Link>
    );
}
