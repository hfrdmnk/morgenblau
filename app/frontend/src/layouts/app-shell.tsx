import type { ReactNode } from 'react';
import { useEffect, useState } from 'react';
import { useLocation } from 'wouter';

import { AppHeader } from '@/components/app-header';
import { ErrorBoundary } from '@/components/error-boundary';
import { ErrorState } from '@/components/status-state';
import { Button } from '@/components/ui/button';
import { useNavigationScrollRestoration } from '@/hooks/use-app-location';
import { useModeShortcuts } from '@/hooks/use-shortcuts';
import { api, ApiError } from '@/lib/api';
import {
    AppProfileContext,
    type AppProfile,
    type AppProfileState,
} from '@/lib/app-profile-context';
import { isAdminPath, PATHS } from '@/lib/paths';

function isAbortError(error: unknown): boolean {
    return error instanceof DOMException && error.name === 'AbortError';
}

function isUnauthorized(error: unknown): boolean {
    return error instanceof ApiError && error.status === 401;
}

async function loadProfile(signal: AbortSignal): Promise<AppProfileState | null> {
    try {
        const profile = await api<AppProfile>('/api/profiles/me', { signal });
        return { kind: 'ready', profile };
    } catch (error) {
        if (isAbortError(error)) {
            return null;
        }
        if (isUnauthorized(error)) {
            window.location.replace(PATHS.login);
            return null;
        }
        return { kind: 'error' };
    }
}

export function AppShell({ children }: { children: ReactNode }) {
    const [attempt, setAttempt] = useState(0);
    const [state, setState] = useState<AppProfileState>({ kind: 'loading' });
    const [location] = useLocation();
    useNavigationScrollRestoration();
    useModeShortcuts();

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

function AdminBanner() {
    return (
        <div className="admin-banner bg-atmosphere-blue px-5 pb-2 pt-[calc(0.5rem+env(safe-area-inset-top))] text-center text-sm font-medium text-white sm:px-8">
            Admin only
        </div>
    );
}
