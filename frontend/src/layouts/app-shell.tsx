import type { ReactNode } from 'react';
import { useEffect, useState } from 'react';

import { ErrorState, LoadingState } from '@/components/status-state';
import { Button } from '@/components/ui/button';
import {
    Popover,
    PopoverContent,
    PopoverDescription,
    PopoverHeader,
    PopoverTitle,
    PopoverTrigger,
} from '@/components/ui/popover';
import { api, ApiError } from '@/lib/api';
import { PATHS } from '@/lib/paths';

type Profile = {
    did: string;
    handle: string;
    displayName: string | null;
    avatar: string | null;
};

type ProfileState =
    | { kind: 'loading' }
    | { kind: 'ready'; profile: Profile }
    | { kind: 'error' };

function isAbortError(error: unknown): boolean {
    return error instanceof DOMException && error.name === 'AbortError';
}

function isUnauthorized(error: unknown): boolean {
    return error instanceof ApiError && error.status === 401;
}

async function loadProfile(signal: AbortSignal): Promise<ProfileState | null> {
    try {
        const profile = await api<Profile>('/api/profiles/me', { signal });
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

function profileInitial(profile: Profile): string {
    const source = profile.displayName?.trim() || profile.handle || profile.did;
    return source.charAt(0).toLocaleUpperCase();
}

function ProfileAvatar({ profile }: { profile: Profile }) {
    return (
        <span className="flex size-8 items-center justify-center overflow-hidden rounded-full bg-secondary text-sm font-medium text-secondary-foreground">
            {profile.avatar ? (
                <img
                    src={profile.avatar}
                    alt=""
                    className="size-full object-cover"
                    referrerPolicy="no-referrer"
                />
            ) : (
                profileInitial(profile)
            )}
        </span>
    );
}

function AccountMenu({ profile }: { profile: Profile }) {
    const accountName = profile.displayName?.trim() || `@${profile.handle}`;

    return (
        <Popover>
            <PopoverTrigger
                render={
                    <Button
                        variant="ghost"
                        size="icon"
                        className="rounded-full p-1"
                        aria-label="Open account menu"
                    />
                }
            >
                <ProfileAvatar profile={profile} />
            </PopoverTrigger>
            <PopoverContent align="end" className="w-64">
                <PopoverHeader>
                    <PopoverTitle className="truncate">{accountName}</PopoverTitle>
                    <PopoverDescription className="truncate">
                        @{profile.handle}
                    </PopoverDescription>
                </PopoverHeader>
                <form method="POST" action={PATHS.oauthLogout}>
                    <Button type="submit" variant="secondary" className="w-full">
                        Log out
                    </Button>
                </form>
            </PopoverContent>
        </Popover>
    );
}

function AuthenticatedShell({
    children,
    profile,
}: {
    children: ReactNode;
    profile: Profile;
}) {
    return (
        <div className="min-h-dvh">
            <header className="flex h-20 items-center justify-end px-6 sm:px-10">
                <AccountMenu profile={profile} />
            </header>
            {children}
        </div>
    );
}

export function AppShell({ children }: { children: ReactNode }) {
    const [attempt, setAttempt] = useState(0);
    const [state, setState] = useState<ProfileState>({ kind: 'loading' });

    useEffect(() => {
        const controller = new AbortController();

        void loadProfile(controller.signal).then((nextState) => {
            if (nextState) {
                setState(nextState);
            }
        });

        return () => controller.abort();
    }, [attempt]);

    if (state.kind === 'loading') {
        return (
            <main className="grid min-h-dvh place-items-center">
                <LoadingState title="Opening your newspaper" />
            </main>
        );
    }

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

    return <AuthenticatedShell profile={state.profile}>{children}</AuthenticatedShell>;
}
