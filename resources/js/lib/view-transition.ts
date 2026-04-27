import { router } from '@inertiajs/react';

export function visitWithTransition(href: string) {
    const prefersReduced =
        typeof window !== 'undefined' &&
        window.matchMedia('(prefers-reduced-motion: reduce)').matches;

    if (
        typeof document === 'undefined' ||
        !document.startViewTransition ||
        prefersReduced
    ) {
        router.visit(href);

        return;
    }

    document.startViewTransition(
        () =>
            new Promise<void>((resolve) => {
                router.visit(href, { onFinish: () => resolve() });
            }),
    );
}
