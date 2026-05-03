<?php

namespace App\Listeners;

use App\Models\User;
use Illuminate\Support\Facades\Auth;
use Revolution\Bluesky\Events\OAuthSessionUpdated;

class PersistOAuthSession
{
    public function handle(OAuthSessionUpdated $event): void
    {
        $did = $event->session->did();

        if (empty($did)) {
            return;
        }

        $user = User::find($did);

        if ($user === null) {
            return;
        }

        $refresh = $event->session->refresh();
        $iss = $event->session->issuer();

        // Route through the model so the 'refresh_token' encrypted cast applies.
        $user->update([
            'refresh_token' => $refresh,
            'iss' => $iss,
        ]);

        session()->put('bluesky_session', $event->session->toArray());

        // Keep the currently-authenticated User instance in sync. Without this,
        // tokenForBluesky() merges the now-stale in-memory refresh_token back
        // over the rotated one, the response middleware writes that stale value
        // back here, and the consumed token poisons the next refresh.
        $current = Auth::user();
        if ($current instanceof User && $current->getKey() === $did) {
            $current->forceFill([
                'refresh_token' => $refresh,
                'iss' => $iss,
            ])->syncOriginal();
        }
    }
}
