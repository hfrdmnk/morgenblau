import { useEffect, useState } from 'react';
import { useSearch } from 'wouter/use-browser-location';

import { AlertIcon } from '@/components/icons';
import { PersonAvatar } from '@/components/person-avatar';
import { Button } from '@/components/ui/button';
import { Autocomplete, ComboboxContent, ComboboxInput, ComboboxItem, ComboboxList } from '@/components/ui/combobox';
import { Input } from '@/components/ui/input';
import { useDocumentTitle } from '@/hooks/use-document-title';
import { api } from '@/lib/api';
import { PATHS } from '@/lib/paths';

const LOGIN_ERRORS: Record<string, string> = {
    handle: 'Couldn’t start sign-in with that handle. Check the spelling and try again.',
    callback: 'Sign-in didn’t finish. Try again.',
};

const TYPEAHEAD_SEARCH = 'https://typeahead.waow.tech/xrpc/tech.waow.typeahead.searchActors';

export function Login() {
    const [pending, setPending] = useState(false);
    useDocumentTitle('Sign in');

    // A page restored from the back-forward cache keeps its pending state, so Back would leave the button stuck.
    useEffect(() => {
        const reset = () => setPending(false);
        window.addEventListener('pageshow', reset);
        return () => window.removeEventListener('pageshow', reset);
    }, []);

    return (
        <main className="grid min-h-dvh place-items-center px-6 py-12">
            <div className="w-full max-w-sm space-y-10">
                <header className="space-y-5">
                    <img src="/favicon.svg" alt="" className="size-10" />
                    <div className="space-y-2">
                        <p className="text-sm font-medium text-muted-foreground">
                            Morgenblau
                        </p>
                        <h1 className="text-3xl font-medium tracking-tight">
                            Sign in with your Atmosphere account
                        </h1>
                        <p className="text-sm text-muted-foreground">
                            Your account is your identity across AT Protocol apps.
                        </p>
                    </div>
                </header>

                <form method="POST" action={PATHS.oauthLogin} className="space-y-4" onSubmit={() => setPending(true)}>
                    <HandleField />
                    <Button type="submit" className="w-full" disabled={pending}>
                        {pending ? 'Signing in…' : 'Continue'}
                    </Button>
                </form>
                {import.meta.env.DEV && <DevelopmentLogin />}
            </div>
        </main>
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
        <div className="space-y-2">
            <label htmlFor="handle" className="text-sm font-medium">
                Handle
            </label>
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
                            <ComboboxItem key={actor.did} value={actor} className="py-1.5">
                                <PersonAvatar className="size-7" seed={actor.did} src={actor.avatar} />
                                <span className="min-w-0">
                                    <span className="block truncate font-medium">{actor.displayName || actor.handle}</span>
                                    <span className="block truncate text-subtle-foreground">@{actor.handle}</span>
                                </span>
                            </ComboboxItem>
                        )}
                    </ComboboxList>
                </ComboboxContent>
            </Autocomplete>
            {error ? (
                <p id="handle-error" role="alert" className="flex items-start gap-2 text-sm text-destructive">
                    <AlertIcon className="mt-0.5 size-4 shrink-0" />
                    {error}
                </p>
            ) : null}
        </div>
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
        <aside className="space-y-3 rounded-xl bg-muted p-4" aria-label="Development login">
            <p className="text-sm font-medium">Development account</p>
            <p className="text-sm text-muted-foreground">
                Uses the server’s configured account. Changes write to its real PDS.
            </p>
            <Button className="w-full" disabled={pending} onClick={login}>
                {pending ? 'Signing in…' : 'Log me in'}
            </Button>
            {error && (
                <p role="alert" className="text-sm text-destructive">
                    Could not sign in. Check the server credentials and try again.
                </p>
            )}
        </aside>
    );
}
