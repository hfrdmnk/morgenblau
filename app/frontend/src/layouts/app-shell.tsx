import type { ReactNode } from 'react';
import { useEffect, useState } from 'react';
import { useLocation } from 'wouter';

import { AppHeader } from '@/components/app-header';
import { ErrorBoundary } from '@/components/error-boundary';
import { ErrorState } from '@/components/status-state';
import { Button } from '@/components/ui/button';
import { useNavigationScrollRestoration } from '@/hooks/use-app-location';
import { useCommands } from '@/hooks/use-commands';
import { useModeShortcuts } from '@/hooks/use-shortcuts';
import { AppProfileContext, loadProfile, type AppProfileState } from '@/lib/app-profile-context';
import { GO_TO, type Command } from '@/lib/commands';
import { isAdminPath, PATHS } from '@/lib/paths';

export function AppShell({ children }: { children: ReactNode }) {
    const [attempt, setAttempt] = useState(0);
    const [state, setState] = useState<AppProfileState>({ kind: 'loading' });
    const [location, navigate] = useLocation();
    useNavigationScrollRestoration();
    useModeShortcuts();
    useCommands(adminCommands(state, navigate));

    useEffect(() => {
        const controller = new AbortController();

        void loadProfile(controller.signal).then((nextState) => {
            if (nextState) {
                setState(nextState);
            }
        });

        return () => controller.abort();
    }, [attempt]);

    if (state.kind === 'error') {
        return (
            <main className="grid min-h-dvh place-items-center">
                <ErrorState
                    title="Couldn’t load your account"
                    description="Try again in a moment."
                    action={
                        <Button
                            variant="secondary"
                            onClick={() => {
                                setState({ kind: 'loading' });
                                setAttempt((current) => current + 1);
                            }}
                        >
                            Try again
                        </Button>
                    }
                />
            </main>
        );
    }

    return (
        <AppProfileContext.Provider value={state}>
            <div className="app-shell min-h-dvh bg-background">
                <a
                    className="sr-only rounded-md bg-popover px-4 py-2 text-sm font-medium shadow-popover focus:not-sr-only focus:fixed focus:top-[calc(1rem+env(safe-area-inset-top))] focus:left-4 focus:z-50"
                    href="#main"
                >
                    Skip to content
                </a>
                {isAdminPath(location) ? <AdminBanner /> : null}
                <AppHeader />
                <ErrorBoundary key={location}>{children}</ErrorBoundary>
            </div>
        </AppProfileContext.Provider>
    );
}

function adminCommands(state: AppProfileState, navigate: (to: string) => void): Command[] {
    if (state.kind !== 'ready' || !state.profile.isAdmin) return [];
    return [{ id: 'go:admin', group: GO_TO, label: 'Admin', keywords: ['usage'], run: () => navigate(PATHS.admin) }];
}

function AdminBanner() {
    return (
        <div className="admin-banner bg-atmosphere-blue px-5 pb-2 pt-[calc(0.5rem+env(safe-area-inset-top))] text-center text-sm font-medium text-white sm:px-8">
            Admin only
        </div>
    );
}
