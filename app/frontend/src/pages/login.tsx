import { useEffect, useState } from 'react';
import { useSearch } from 'wouter/use-browser-location';

import { AlertIcon } from '@/components/icons';
import { PersonAvatar } from '@/components/person-avatar';
import { Button } from '@/components/ui/button';
import { Autocomplete, ComboboxContent, ComboboxInput, ComboboxItem, ComboboxList } from '@/components/ui/combobox';
import { Input } from '@/components/ui/input';
import { Frame } from '@/design/frame';
import { Text } from '@/design/text';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { api, clearOfflineCopies } from '@/lib/api';
import { PATHS } from '@/lib/paths';

const LOGIN_ERRORS: Record<string, string> = {
    handle: 'Couldn’t start sign-in with that handle. Check the spelling and try again.',
    callback: 'Sign-in didn’t finish. Try again.',
};

const TYPEAHEAD_SEARCH = 'https://typeahead.waow.tech/xrpc/tech.waow.typeahead.searchActors';

export function Login() {
    const [pending, setPending] = useState(false);
    useDocumentTitle('Sign in');

    // Logging out and every sign-in land here, so one reader's offline copies never reach the next.
    useEffect(() => {
        void clearOfflineCopies();
    }, []);

    // A page restored from the back-forward cache keeps its pending state, so Back would leave the button stuck.
    useEffect(() => {
        const reset = () => setPending(false);
        window.addEventListener('pageshow', reset);
        return () => window.removeEventListener('pageshow', reset);
    }, []);

    return (
        <Frame as="main" minHeight="screen" align="center" justify="center" paddingX="2xl" paddingY="5xl">
            <Frame width="full" maxWidth="sm" gap="4xl">
                <Frame as="header" gap="xl">
                    <img src="/favicon.svg" alt="" width={40} height={40} />
                    <Frame gap="s">
                        <Text size="sm" weight="medium" tone="muted">
                            Morgenblau
                        </Text>
                        <Text as="h1" size="3xl" weight="medium" tracking="tight">
                            Sign in with your Atmosphere account
                        </Text>
                        <Text size="sm" tone="muted">
                            Your account is your identity across AT Protocol apps.
                        </Text>
                    </Frame>
                </Frame>

                <Frame as="form" method="POST" action={PATHS.oauthLogin} gap="l" onSubmit={() => setPending(true)}>
                    <HandleField />
                    <Button type="submit" disabled={pending}>
                        {pending ? 'Signing in…' : 'Continue'}
                    </Button>
                </Frame>
                {import.meta.env.DEV && <DevelopmentLogin />}
            </Frame>
        </Frame>
    );
}

// A failed sign-in returns here with the handle and an error code, so the field comes back filled in and explains what went wrong.
function HandleField() {
    const { error: code = '', handle = '' } = Object.fromEntries(new URLSearchParams(useSearch()));
    const error = LOGIN_ERRORS[code];
    const [value, setValue] = useState(handle);
    // Only typing searches, so a prefilled handle or a picked suggestion costs no request.
    const [search, setSearch] = useState('');
    const [open, setOpen] = useState(false);
    const actors = useActorSuggestions(search);
    // An open popup hides the rest of the page from assistive tech, Continue included, so it stays closed without suggestions.
    const shown = open && actors.length > 0;
    return (
        <Frame gap="s">
            <Text as="label" htmlFor="handle" size="sm" weight="medium">
                Handle
            </Text>
            <Autocomplete
                filter={null}
                items={actors}
                itemToStringValue={(actor: Actor) => actor.handle}
                onOpenChange={setOpen}
                onValueChange={(next, details) => {
                    setValue(next);
                    if (details.reason === 'input-change') setSearch(next);
                }}
                open={shown}
                value={value}
            >
                <ComboboxInput
                    id="handle"
                    name="handle"
                    type="text"
                    autoComplete="username"
                    autoCapitalize="none"
                    autoFocus={!window.matchMedia('(pointer: coarse)').matches}
                    aria-describedby={error && 'handle-error'}
                    aria-invalid={Boolean(error)}
                    // Base UI only reports closing while the popup is mounted, so leaving the field closes it here, before late results can open it.
                    onBlur={() => setOpen(false)}
                    onKeyDown={(event) => {
                        if (event.key !== 'Escape' || shown) return;
                        // Base UI clears the input on Escape when the popup is closed, which would wipe a typed or prefilled handle.
                        event.preventBaseUIHandler();
                        setOpen(false);
                    }}
                    inputMode="text"
                    placeholder="you.example"
                    render={<Input />}
                    required
                    spellCheck={false}
                />
                <ComboboxContent>
                    <ComboboxList>
                        {(actor: Actor) => (
                            <ComboboxItem key={actor.did} value={actor} padded>
                                <PersonAvatar size="sm" seed={actor.did} src={actor.avatar} />
                                <Frame as="span" minWidth={0}>
                                    <Text as="span" truncate weight="medium">
                                        {actor.displayName || actor.handle}
                                    </Text>
                                    <Text as="span" truncate tone="subtle">
                                        @{actor.handle}
                                    </Text>
                                </Frame>
                            </ComboboxItem>
                        )}
                    </ComboboxList>
                </ComboboxContent>
            </Autocomplete>
            {error ? (
                <Frame id="handle-error" role="alert" direction="row" align="start" gap="s">
                    <Frame paddingTop="3xs">
                        <AlertIcon size="md" tone="destructive" />
                    </Frame>
                    <Text size="sm" tone="destructive">
                        {error}
                    </Text>
                </Frame>
            ) : null}
        </Frame>
    );
}

type Actor = { did: string; handle: string; displayName?: string; avatar?: string };

// Typeahead allows 60 requests a minute per IP, so searches wait for a pause in typing.
const SUGGESTION_DELAY_MS = 350;

// Suggestions are optional: a failed search leaves the field working as a plain handle input.
function useActorSuggestions(input: string) {
    const [actors, setActors] = useState<Actor[]>([]);
    const query = input.trim().replace(/^@/, '');
    useEffect(() => {
        if (!query) return;
        const controller = new AbortController();
        const timer = window.setTimeout(() => {
            const params = new URLSearchParams({ q: query, limit: '5' });
            void api<{ actors?: Actor[] }>(`${TYPEAHEAD_SEARCH}?${params}`, { signal: controller.signal })
                .then((result) => setActors(result.actors ?? []))
                .catch(() => {
                    if (!controller.signal.aborted) setActors([]);
                });
        }, SUGGESTION_DELAY_MS);
        return () => {
            window.clearTimeout(timer);
            controller.abort();
        };
    }, [query]);
    return query ? actors : [];
}

function DevelopmentLogin() {
    const [enabled, setEnabled] = useState(false);
    const [pending, setPending] = useState(false);
    const [error, setError] = useState(false);

    useEffect(() => {
        const controller = new AbortController();
        void api<{ enabled: boolean }>('/__dev/login', { signal: controller.signal })
            .then((result) => setEnabled(result.enabled))
            .catch(() => {});
        return () => controller.abort();
    }, []);

    async function login() {
        setPending(true);
        setError(false);
        try {
            await api('/__dev/login', { method: 'POST' });
            window.location.assign(PATHS.digest);
        } catch {
            setError(true);
            setPending(false);
        }
    }

    if (!enabled) return null;

    return (
        <Frame as="aside" gap="m" radius="xl" background="muted" padding="l" aria-label="Development login">
            <Text size="sm" weight="medium">
                Development account
            </Text>
            <Text size="sm" tone="muted">
                Uses the server’s configured account. Changes write to its real PDS.
            </Text>
            <Button disabled={pending} onClick={login}>
                {pending ? 'Signing in…' : 'Log me in'}
            </Button>
            {error && (
                <Text role="alert" size="sm" tone="destructive">
                    Could not sign in. Check the server credentials and try again.
                </Text>
            )}
        </Frame>
    );
}
