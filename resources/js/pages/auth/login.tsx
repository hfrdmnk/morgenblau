import { Form, Head } from '@inertiajs/react';
import InputError from '@/components/input-error';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import AuthLayout from '@/layouts/auth-layout';

export default function Login() {
    return (
        <AuthLayout
            title="Sign in with Bluesky"
            description="Enter your handle to continue"
        >
            <Head title="Log in" />

            <Form action="/login" method="post" className="space-y-6">
                {({ processing, errors }) => (
                    <>
                        <div className="grid gap-2">
                            <Label htmlFor="handle">Handle</Label>
                            <Input
                                id="handle"
                                type="text"
                                name="handle"
                                autoComplete="username"
                                autoFocus
                                placeholder="alice.bsky.social"
                                inputMode="email"
                                spellCheck={false}
                            />
                            <InputError message={errors.handle} />
                        </div>

                        <Button
                            type="submit"
                            disabled={processing}
                            className="w-full"
                            data-test="login-submit"
                        >
                            Continue
                        </Button>
                    </>
                )}
            </Form>
        </AuthLayout>
    );
}
