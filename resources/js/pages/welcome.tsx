import { Head, usePage } from '@inertiajs/react';
import { useEffect } from 'react';
import AppLogoIcon from '@/components/app-logo-icon';
import { Button } from '@/components/ui/button';
import { Window } from '@/components/window';
import { visitWithTransition } from '@/lib/view-transition';
import { dashboard, login } from '@/routes';

export default function Welcome() {
    const { auth } = usePage().props;
    const target = auth.user ? dashboard().url : login().url;

    useEffect(() => {
        const handler = (event: KeyboardEvent) => {
            if (event.key !== 'Enter') {
                return;
            }

            const target = event.target as HTMLElement | null;

            if (
                target &&
                target.closest('input, textarea, [contenteditable]')
            ) {
                return;
            }

            event.preventDefault();
            visitWithTransition(auth.user ? dashboard().url : login().url);
        };
        window.addEventListener('keydown', handler);

        return () => window.removeEventListener('keydown', handler);
    }, [auth.user]);

    return (
        <>
            <Head title="Morgenblau" />
            <div className="min-h-svh bg-background p-6">
                <Window
                    variant="sunrise"
                    className="flex min-h-[calc(100svh-3rem)] items-center justify-center"
                >
                    <div className="flex flex-col items-center gap-8 px-6 text-center text-white">
                        <AppLogoIcon
                            className="size-16"
                            style={{ viewTransitionName: 'brand-logo' }}
                        />
                        <div className="space-y-3">
                            <h1>Morgenblau</h1>
                            <p className="max-w-md text-base text-balance">
                                A morning edition of the open web.
                            </p>
                            <p className="max-w-md text-sm font-light text-pretty text-white/80">
                                Pick your sources. Read once a day. Close the
                                tab.
                            </p>
                        </div>
                        <Button
                            variant="ghost-on-gradient"
                            className="text-base"
                            onClick={() => visitWithTransition(target)}
                        >
                            Enter
                        </Button>
                    </div>
                </Window>
            </div>
        </>
    );
}
