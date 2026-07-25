import { Link } from 'wouter';

import type { ShareTargetPresentation } from '@/lib/share-target';

export function RowDivider() {
    return <li aria-hidden className="mx-6 border-t border-border" />;
}

export const ROW_CLASS =
    'relative flex items-start gap-3 px-6 py-5 transition-colors duration-200 ease-out has-[a:focus-visible]:outline-1 has-[a:focus-visible]:-outline-offset-2 has-[a:focus-visible]:outline-solid has-[a:focus-visible]:outline-ring';

// Absolutely-positioned click target so the whole row activates while leaving room for the row's own interactive children above it.
export function RowOverlayLink({ target }: { target: ShareTargetPresentation }) {
    if (!target.href) return null;
    if (target.external) {
        return (
            <a
                href={target.href}
                target="_blank"
                rel="noopener noreferrer"
                aria-label={target.label}
                className="absolute inset-0 outline-none"
            />
        );
    }
    return (
        <Link
            href={target.href}
            aria-label={target.label}
            className="absolute inset-0 outline-none"
        />
    );
}

export function ShareComment({ comment }: { comment: string | undefined }) {
    if (!comment) return null;
    return (
        <p className="mt-1 line-clamp-2 text-sm font-light text-muted-foreground">
            {comment}
        </p>
    );
}
