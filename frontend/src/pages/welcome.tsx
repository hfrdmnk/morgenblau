import { useLocation } from 'wouter';

import { AppLogoIcon } from '@/components/app-logo-icon';
import { Button } from '@/components/ui/button';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { PATHS } from '@/lib/paths';

export function Welcome() {
    useDocumentTitle();
    const [, navigate] = useLocation();

    return (
        <div className="min-h-svh bg-background p-6">
            <div className="flex min-h-[calc(100svh-3rem)] items-center justify-center overflow-hidden rounded-xl bg-sunrise shadow-[inset_0_0_0_1px_rgba(255,255,255,0.4)]">
                <div className="flex flex-col items-center gap-8 px-6 text-center text-white">
                    <AppLogoIcon className="size-16" />
                    <div className="space-y-3">
                        <h1>Morgenblau</h1>
                        <p className="max-w-xl text-base text-balance">
                            Your personal daily web-newspaper. Curate the
                            sources you care about and read them at your pace.
                        </p>
                    </div>
                    <Button
                        variant="ghost-on-gradient"
                        className="text-base"
                        onClick={() => navigate(PATHS.login)}
                    >
                        Begin
                    </Button>
                </div>
            </div>
        </div>
    );
}
