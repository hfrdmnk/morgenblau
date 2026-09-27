import { useEffect, useState } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { api } from '@/lib/api';
import { PATHS } from '@/lib/paths';

export function Login() {
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

                <form method="POST" action={PATHS.oauthLogin} className="space-y-4">
                    <div className="space-y-2">
                        <label htmlFor="handle" className="text-sm font-medium">
                            Handle
                        </label>
                        <Input
                            id="handle"
                            name="handle"
                            type="text"
                            autoComplete="username"
                            autoCapitalize="none"
                            autoFocus
                            inputMode="text"
                            placeholder="you.example"
                            required
                            spellCheck={false}
                        />
                    </div>
                    <Button type="submit" className="w-full">
                        Continue
                    </Button>
                </form>
                {import.meta.env.DEV && <DevelopmentLogin />}
            </div>
        </main>
    );
}

function DevelopmentLogin() {
    const [enabled, setEnabled] = useState(false);
    const [pending, setPending] = useState(false);
    const [error, setError] = useState(false);

    useEffect(() => {
        const controller = new AbortController();
        void api<{ enabled: boolean }>('/dev/login', { signal: controller.signal })
            .then((result) => setEnabled(result.enabled))
            .catch(() => {});
        return () => controller.abort();
    }, []);

    async function login() {
        setPending(true);
        setError(false);
        try {
            await api('/dev/login', { method: 'POST' });
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
