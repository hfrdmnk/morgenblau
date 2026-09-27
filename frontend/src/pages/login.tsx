import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
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
            </div>
        </main>
    );
}
