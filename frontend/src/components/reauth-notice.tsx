import { PATHS } from '@/lib/paths';

// Native anchor: reauth exits the authed shell, which app.tsx assumes is a full server round trip.
export function ReauthNotice() {
    return (
        <p role="status" className="text-sm font-light text-muted-foreground">
            Subscribing via ATProto needs one extra permission.{' '}
            <a
                href={PATHS.login}
                className="text-primary underline underline-offset-4"
            >
                Sign in again
            </a>{' '}
            to enable it.
        </p>
    );
}
