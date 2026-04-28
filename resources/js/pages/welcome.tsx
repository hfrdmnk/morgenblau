import { router, usePage } from '@inertiajs/react';
import { useEffect } from 'react';
import AppLogoIcon from '@/components/app-logo-icon';
import { Button } from '@/components/ui/button';
import { Window } from '@/components/window';
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
            router.visit(auth.user ? dashboard().url : login().url);
        };
        window.addEventListener('keydown', handler);

        return () => window.removeEventListener('keydown', handler);
    }, [auth.user]);

    return (
        <>
            <div className="min-h-svh bg-background p-6">
                <Window
                    variant="sunrise"
                    className="flex min-h-[calc(100svh-3rem)] items-center justify-center"
                >
                    <div className="flex flex-col items-center gap-8 px-6 text-center text-white">
                        <AppLogoIcon className="size-16" />
                        <div className="space-y-3">
                            <h1>Morgenblau</h1>
                            <p className="max-w-xl text-base text-balance">
                                A reading room and a quiet square on the open
                                web. Read what you follow, post what you find,
                                see what others are reading.
                            </p>
                        </div>
                        <Button
                            variant="ghost-on-gradient"
                            className="text-base"
                            onClick={() => router.visit(target)}
                        >
                            Begin
                        </Button>
                    </div>
                </Window>
            </div>
        </>
    );
}
