import { AppLogoIcon } from '@/components/app-logo-icon';
import { Button } from '@/components/ui/button';
import { Window } from '@/components/window';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { PATHS } from '@/lib/paths';

export function Welcome() {
    useDocumentTitle();

    return (
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
                            A reading room and a quiet square on the open web.
                            Read what you follow, post what you find, see what
                            others are reading.
                        </p>
                    </div>
                    <Button
                        variant="ghost-on-gradient"
                        className="text-base"
                        onClick={() => window.location.assign(PATHS.login)}
                    >
                        Begin
                    </Button>
                </div>
            </Window>
        </div>
    );
}
