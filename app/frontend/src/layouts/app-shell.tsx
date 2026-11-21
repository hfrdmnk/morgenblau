import type { ReactNode } from 'react';
import { useEffect, useState } from 'react';
import { useLocation } from 'wouter';

import { AppHeader } from '@/components/app-header';
import { ErrorBoundary } from '@/components/error-boundary';
import { ErrorState } from '@/components/status-state';
import { AdminBanner, AppShellLayout } from '@/components/ui/app-chrome';
import { Button } from '@/components/ui/button';
import { Link } from '@/components/ui/link';
import { Frame } from '@/design/frame';
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
            <Frame align="center" as="main" justify="center" minHeight="screen">
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
            </Frame>
        );
    }

    return (
        <AppProfileContext.Provider value={state}>
            <AppShellLayout>
                <Link href="#main" variant="skip">
                    Skip to content
                </Link>
                {isAdminPath(location) ? <AdminBanner>Admin only</AdminBanner> : null}
                <AppHeader />
                <ErrorBoundary key={location}>{children}</ErrorBoundary>
            </AppShellLayout>
        </AppProfileContext.Provider>
    );
}

function adminCommands(state: AppProfileState, navigate: (to: string) => void): Command[] {
    if (state.kind !== 'ready' || !state.profile.isAdmin) return [];
    return [{ id: 'go:admin', group: GO_TO, label: 'Admin', keywords: ['usage'], run: () => navigate(PATHS.admin) }];
}
