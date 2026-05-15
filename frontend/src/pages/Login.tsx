import { Loading03Icon, Login03Icon } from '@hugeicons/core-free-icons';
import { HugeiconsIcon } from '@hugeicons/react';
import { useState } from 'react';
import AuthGoldenLayout from '@/layouts/auth-golden-layout';
import InputError from '@/components/input-error';
import { Button } from '@/components/ui/button';
import {
    InputGroup,
    InputGroupAddon,
    InputGroupInput,
} from '@/components/ui/input-group';
import { Label } from '@/components/ui/label';
import { useDocumentTitle } from '@/hooks/use-document-title';

// Native form submit — no fetch, no preventDefault. The browser must navigate
// to follow the AS redirect that /oauth/login responds with.
export function Login() {
    useDocumentTitle('Sign in');
    const [handle, setHandle] = useState('');
    const [pending, setPending] = useState(false);
    const [error] = useState<string | undefined>(undefined);

    return (
        <AuthGoldenLayout>
            <div className="space-y-8 motion-safe:animate-in motion-safe:duration-200 motion-safe:fade-in">
                <header className="space-y-1">
                    <h1>Find. Read. Share.</h1>
                    <p className="text-sm text-balance text-muted-foreground">
                        A calmer way to be on the open web. Powered by RSS and
                        the AT Protocol.
                    </p>
                </header>

                <form
                    method="POST"
                    action="/oauth/login"
                    onSubmit={() => setPending(true)}
                    className="space-y-3"
                >
                    <div className="space-y-2">
                        <Label htmlFor="handle">
                            Enter with your Atmosphere Account
                        </Label>
                        <div className="flex items-center gap-2">
                            <InputGroup className="flex-1">
                                <InputGroupAddon align="inline-start">
                                    @
                                </InputGroupAddon>
                                <InputGroupInput
                                    id="handle"
                                    name="handle"
                                    type="text"
                                    autoComplete="username"
                                    autoFocus
                                    placeholder="alice.bsky.social"
                                    inputMode="email"
                                    spellCheck={false}
                                    value={handle}
                                    onChange={(e) =>
                                        setHandle(e.target.value)
                                    }
                                    aria-invalid={error ? true : undefined}
                                />
                            </InputGroup>
                            <Button
                                type="submit"
                                size="icon"
                                disabled={pending || !handle.trim()}
                                aria-label="Continue"
                                data-test="login-submit"
                            >
                                {pending ? (
                                    <HugeiconsIcon
                                        icon={Loading03Icon}
                                        className="motion-safe:animate-spin"
                                    />
                                ) : (
                                    <HugeiconsIcon icon={Login03Icon} />
                                )}
                            </Button>
                        </div>
                        <InputError message={error} />
                    </div>

                    <div className="space-y-1 pt-2 text-sm leading-snug font-light text-muted-foreground">
                        <p>New here?</p>
                        <p>
                            An Atmosphere account is your identity (or user
                            name) across Bluesky, Tangled, Leaflet, Grain and
                            other AT Protocol apps.
                        </p>
                    </div>
                </form>
            </div>
        </AuthGoldenLayout>
    );
}
